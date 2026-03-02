package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
)

// OpenAI embedding model dimensions.
var openaiDimensions = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// OpenAIEmbedder implements Embedder using the OpenAI embeddings API.
type OpenAIEmbedder struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewOpenAIEmbedder creates an OpenAI embedder.
// Parameters override defaults; pass empty strings to use defaults.
//   - apiKey (required): API key for authentication
//   - baseURL: Base URL, defaults to https://api.openai.com/v1
//   - model: Model name, defaults to text-embedding-3-small
func NewOpenAIEmbedder(apiKey, baseURL, model string) (*OpenAIEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for openai embedding backend (set api-key in config or OPENAI_API_KEY env)")
	}

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	if model == "" {
		model = "text-embedding-3-small"
	}

	return &OpenAIEmbedder{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, nil
}

func (o *OpenAIEmbedder) Name() string {
	return fmt.Sprintf("openai/%s", o.Model)
}

func (o *OpenAIEmbedder) EmbedDimension() int {
	if dim, ok := openaiDimensions[o.Model]; ok {
		return dim
	}
	return 0
}

func (o *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return results[0], nil
}

func (o *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	type request struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}
	type embData struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}
	type errorResponse struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	type response struct {
		Data  []embData `json:"data"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error,omitempty"`
	}

	body, _ := json.Marshal(request{Input: texts, Model: o.Model})

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embed failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding openai embed response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai embed error: %s", result.Error.Message)
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
