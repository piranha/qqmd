// Package llm provides an interface for LLM operations (embeddings, reranking, query expansion)
// and an Ollama-based implementation.
package llm

import "context"

// Provider is the interface for LLM backends.
type Provider interface {
	// Embed returns a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch returns embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Rerank scores documents against a query. Returns scores in the same order as docs.
	Rerank(ctx context.Context, query string, docs []string) ([]float64, error)

	// ExpandQuery generates expanded query variants for hybrid search.
	ExpandQuery(ctx context.Context, query string) (*ExpandedQuery, error)

	// Name returns the provider name for display.
	Name() string
}

// ExpandedQuery holds the different query forms for hybrid search.
type ExpandedQuery struct {
	Lex  string `json:"lex"`  // BM25 keyword variant
	Vec  string `json:"vec"`  // Semantic search variant
	Hyde string `json:"hyde"` // Hypothetical document variant
}

// FormatDocForEmbedding formats a document for embedding input.
func FormatDocForEmbedding(title, text string) string {
	if title != "" {
		return "title: " + title + " | text: " + text
	}
	return "text: " + text
}

// FormatQueryForEmbedding formats a search query for embedding input.
func FormatQueryForEmbedding(query string) string {
	return "task: search result | query: " + query
}
