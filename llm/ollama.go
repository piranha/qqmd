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

// --- Ollama Embedder ---

// OllamaEmbedder implements Embedder using a local Ollama instance.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
}

// NewOllamaEmbedder creates an Ollama embedder.
// Parameters override defaults; pass empty strings to use defaults.
// Defaults: baseURL=OLLAMA_HOST or http://localhost:11434, model=nomic-embed-text
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = os.Getenv("OLLAMA_HOST")
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbedder{BaseURL: baseURL, Model: model}
}

func (o *OllamaEmbedder) Name() string {
	return fmt.Sprintf("ollama/%s", o.Model)
}

func (o *OllamaEmbedder) EmbedDimension() int { return 0 }

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return result[0], nil
}

func (o *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	type request struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	type response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	body, _ := json.Marshal(request{Model: o.Model, Input: texts})
	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w (is Ollama running?)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embed response: %w", err)
	}

	return result.Embeddings, nil
}

// --- Ollama Chat Provider ---

// OllamaChatProvider implements ChatProvider using a local Ollama instance.
type OllamaChatProvider struct {
	BaseURL string
	Model   string
}

func NewOllamaChatProvider() *OllamaChatProvider {
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := os.Getenv("QQMD_CHAT_MODEL")
	if model == "" {
		model = "qwen3:0.6b"
	}
	return &OllamaChatProvider{BaseURL: baseURL, Model: model}
}

func (o *OllamaChatProvider) Name() string {
	return fmt.Sprintf("ollama/%s", o.Model)
}

func (o *OllamaChatProvider) Rerank(ctx context.Context, query string, docs []string) ([]float64, error) {
	if len(docs) == 0 {
		return nil, nil
	}

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

	type request struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
		Format string `json:"format"`
	}
	type response struct {
		Response string `json:"response"`
	}

	body, _ := json.Marshal(request{
		Model:  o.Model,
		Prompt: sb.String(),
		Stream: false,
		Format: "json",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama rerank failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding rerank response: %w", err)
	}

	// Parse scores from JSON response
	var scores []float64
	if err := json.Unmarshal([]byte(result.Response), &scores); err != nil {
		// Try parsing as object with scores key
		var obj map[string][]float64
		if err2 := json.Unmarshal([]byte(result.Response), &obj); err2 == nil {
			for _, v := range obj {
				scores = v
				break
			}
		}
		if scores == nil {
			// Return uniform scores as fallback
			scores = make([]float64, len(docs))
			for i := range scores {
				scores[i] = 5.0
			}
		}
	}

	// Normalize to 0-1
	for i := range scores {
		scores[i] = scores[i] / 10.0
		if scores[i] < 0 {
			scores[i] = 0
		}
		if scores[i] > 1 {
			scores[i] = 1
		}
	}

	// Pad if needed
	for len(scores) < len(docs) {
		scores = append(scores, 0.5)
	}

	return scores[:len(docs)], nil
}

func (o *OllamaChatProvider) ExpandQuery(ctx context.Context, query string) (*ExpandedQuery, error) {
	prompt := fmt.Sprintf(`Expand this search query into three forms for hybrid search.
Output a JSON object with keys "lex", "vec", "hyde":
- lex: keyword-focused version for BM25 text search
- vec: natural language version for semantic vector search
- hyde: a hypothetical document that would perfectly answer the query (1-2 sentences)

Query: %s`, query)

	type request struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
		Format string `json:"format"`
	}
	type response struct {
		Response string `json:"response"`
	}

	body, _ := json.Marshal(request{
		Model:  o.Model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama expand request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama expand failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding expand response: %w", err)
	}

	var expanded ExpandedQuery
	if err := json.Unmarshal([]byte(result.Response), &expanded); err != nil {
		// Fallback: use original query for all forms
		return &ExpandedQuery{Lex: query, Vec: query, Hyde: query}, nil
	}

	return &expanded, nil
}

// RerankResults reranks search results using the provider and returns them sorted by new scores.
func RerankResults(ctx context.Context, provider ChatProvider, query string, results []struct {
	Doc   string
	Index int
}) ([]int, error) {
	if len(results) == 0 {
		return nil, nil
	}

	docs := make([]string, len(results))
	for i, r := range results {
		docs[i] = r.Doc
	}

	scores, err := provider.Rerank(ctx, query, docs)
	if err != nil {
		// On failure, return original order
		indices := make([]int, len(results))
		for i, r := range results {
			indices[i] = r.Index
		}
		return indices, nil
	}

	type ranked struct {
		index int
		score float64
	}
	var ranked_results []ranked
	for i, s := range scores {
		ranked_results = append(ranked_results, ranked{index: results[i].Index, score: s})
	}
	sort.Slice(ranked_results, func(i, j int) bool {
		return ranked_results[i].score > ranked_results[j].score
	})

	indices := make([]int, len(ranked_results))
	for i, r := range ranked_results {
		indices[i] = r.index
	}
	return indices, nil
}
