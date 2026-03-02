package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

// ManagedProvider runs local llama-server subprocesses to serve models directly.
// It auto-downloads the llama-server binary and GGUF models from HuggingFace.
type ManagedProvider struct {
	binary      string
	embedModel  string
	chatModel   string
	embedServer *Server
	chatServer  *Server
}

// NewManagedProvider creates a new managed provider, ensuring binaries and models
// are available (downloading if necessary). Servers are started lazily on first use.
func NewManagedProvider() (*ManagedProvider, error) {
	binary, err := EnsureLlamaServer()
	if err != nil {
		return nil, fmt.Errorf("llama-server: %w", err)
	}

	embedModel, err := EnsureEmbedModel()
	if err != nil {
		return nil, fmt.Errorf("embed model: %w", err)
	}

	chatModel, err := EnsureChatModel()
	if err != nil {
		return nil, fmt.Errorf("chat model: %w", err)
	}

	return &ManagedProvider{
		binary:     binary,
		embedModel: embedModel,
		chatModel:  chatModel,
	}, nil
}

// NewManagedEmbedOnly creates a provider that only downloads the embedding model.
// Chat features (rerank, expand) will return errors.
func NewManagedEmbedOnly() (*ManagedProvider, error) {
	binary, err := EnsureLlamaServer()
	if err != nil {
		return nil, fmt.Errorf("llama-server: %w", err)
	}

	embedModel, err := EnsureEmbedModel()
	if err != nil {
		return nil, fmt.Errorf("embed model: %w", err)
	}

	return &ManagedProvider{
		binary:     binary,
		embedModel: embedModel,
	}, nil
}

func (m *ManagedProvider) Name() string {
	return "managed llama-server"
}

func (m *ManagedProvider) ensureEmbedServer() error {
	if m.embedServer != nil && m.embedServer.Running() {
		return nil
	}
	var err error
	m.embedServer, err = StartServer(ServerConfig{
		Binary:    m.binary,
		Model:     m.embedModel,
		Embedding: true,
		CtxSize:   512,
		GPULayers: -1,
	})
	return err
}

func (m *ManagedProvider) ensureChatServer() error {
	if m.chatModel == "" {
		return fmt.Errorf("chat model not configured")
	}
	if m.chatServer != nil && m.chatServer.Running() {
		return nil
	}
	var err error
	m.chatServer, err = StartServer(ServerConfig{
		Binary:    m.binary,
		Model:     m.chatModel,
		Embedding: false,
		CtxSize:   2048,
		GPULayers: -1,
	})
	return err
}

// Close stops all managed server processes.
func (m *ManagedProvider) Close() {
	if m.embedServer != nil {
		m.embedServer.Stop()
		m.embedServer = nil
	}
	if m.chatServer != nil {
		m.chatServer.Stop()
		m.chatServer = nil
	}
}

// --- Provider interface implementation ---

func (m *ManagedProvider) Embed(_ context.Context, text string) ([]float32, error) {
	results, err := m.EmbedBatch(context.Background(), []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return results[0], nil
}

func (m *ManagedProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if err := m.ensureEmbedServer(); err != nil {
		return nil, err
	}

	// Use OpenAI-compatible /v1/embeddings endpoint
	type req struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}
	type embData struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}
	type resp struct {
		Data []embData `json:"data"`
	}

	body, _ := json.Marshal(req{Input: texts, Model: "embed"})
	httpResp, err := http.Post(m.embedServer.BaseURL+"/v1/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("embed failed (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result resp
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embed response: %w", err)
	}

	// Sort by index to maintain input order
	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Index < result.Data[j].Index
	})

	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

func (m *ManagedProvider) Rerank(_ context.Context, query string, docs []string) ([]float64, error) {
	if err := m.ensureChatServer(); err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, nil
	}

	// Use chat completion to score relevance
	var sb strings.Builder
	sb.WriteString("Rate the relevance of each document to the query on a scale of 0-10.\n")
	sb.WriteString("Output ONLY a JSON array of numbers, one per document.\n\n")
	fmt.Fprintf(&sb, "Query: %s\n\n", query)
	for i, doc := range docs {
		text := doc
		if len(text) > 500 {
			text = text[:500]
		}
		fmt.Fprintf(&sb, "Document %d:\n%s\n\n", i+1, text)
	}

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
	}
	type resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	body, _ := json.Marshal(req{
		Model: "chat",
		Messages: []message{
			{Role: "user", Content: sb.String()},
		},
	})

	httpResp, err := http.Post(m.chatServer.BaseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("rerank failed (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result resp
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding rerank response: %w", err)
	}

	if len(result.Choices) == 0 {
		return uniformScores(len(docs)), nil
	}

	content := result.Choices[0].Message.Content
	scores := parseScores(content, len(docs))

	// Normalize to 0-1
	for i := range scores {
		scores[i] = clamp(scores[i]/10.0, 0, 1)
	}
	return scores, nil
}

func (m *ManagedProvider) ExpandQuery(_ context.Context, query string) (*ExpandedQuery, error) {
	if err := m.ensureChatServer(); err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`Expand this search query into three forms for hybrid search.
Output a JSON object with keys "lex", "vec", "hyde":
- lex: keyword-focused version for BM25 text search
- vec: natural language version for semantic vector search
- hyde: a hypothetical document that would perfectly answer the query (1-2 sentences)

Query: %s`, query)

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
	}
	type resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	body, _ := json.Marshal(req{
		Model: "chat",
		Messages: []message{
			{Role: "user", Content: prompt},
		},
	})

	httpResp, err := http.Post(m.chatServer.BaseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("expand request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("expand failed (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result resp
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding expand response: %w", err)
	}

	if len(result.Choices) == 0 {
		return &ExpandedQuery{Lex: query, Vec: query, Hyde: query}, nil
	}

	content := result.Choices[0].Message.Content
	var expanded ExpandedQuery
	if err := json.Unmarshal([]byte(content), &expanded); err != nil {
		// Try to extract JSON from response (model may wrap it in markdown)
		if idx := strings.Index(content, "{"); idx >= 0 {
			if end := strings.LastIndex(content, "}"); end > idx {
				json.Unmarshal([]byte(content[idx:end+1]), &expanded)
			}
		}
		if expanded.Lex == "" {
			return &ExpandedQuery{Lex: query, Vec: query, Hyde: query}, nil
		}
	}

	return &expanded, nil
}

// helpers

func parseScores(content string, n int) []float64 {
	var scores []float64

	// Try direct JSON array
	if err := json.Unmarshal([]byte(content), &scores); err == nil && len(scores) >= n {
		return scores[:n]
	}

	// Try extracting array from response
	if idx := strings.Index(content, "["); idx >= 0 {
		if end := strings.LastIndex(content, "]"); end > idx {
			if err := json.Unmarshal([]byte(content[idx:end+1]), &scores); err == nil && len(scores) >= n {
				return scores[:n]
			}
		}
	}

	// Try parsing as object with scores key
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		for _, v := range obj {
			if err := json.Unmarshal(v, &scores); err == nil && len(scores) >= n {
				return scores[:n]
			}
		}
	}

	return uniformScores(n)
}

func uniformScores(n int) []float64 {
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = 5.0
	}
	return scores
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StatusMessage returns a message about whether models and binaries are available.
func StatusMessage() string {
	var sb strings.Builder

	if p, err := EnsureLlamaServer(); err == nil {
		fmt.Fprintf(&sb, "llama-server: %s\n", p)
	} else {
		fmt.Fprintf(&sb, "llama-server: not available (%v)\n", err)
	}

	embedPath := modelPath(DefaultEmbedModel)
	if fi, err := os.Stat(embedPath); err == nil {
		fmt.Fprintf(&sb, "embed model: %s (%d MB)\n", embedPath, fi.Size()>>20)
	} else {
		fmt.Fprintf(&sb, "embed model: not downloaded (run 'qqmd embed' to download)\n")
	}

	chatPath := modelPath(DefaultChatModel)
	if fi, err := os.Stat(chatPath); err == nil {
		fmt.Fprintf(&sb, "chat model: %s (%d MB)\n", chatPath, fi.Size()>>20)
	} else {
		fmt.Fprintf(&sb, "chat model: not downloaded (run 'qqmd query' to download)\n")
	}

	return sb.String()
}

func modelPath(m ModelDef) string {
	return fmt.Sprintf("%s/%s", ModelsDir(), m.Filename)
}
