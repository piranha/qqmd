// Package llm provides an interface for LLM operations (embeddings, reranking, query expansion)
// with a managed llama-server backend that auto-downloads models.
package llm

import (
	"context"
	"fmt"
	"os"
)

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

// Closeable is optionally implemented by providers that need cleanup.
type Closeable interface {
	Close()
}

// ExpandedQuery holds the different query forms for hybrid search.
type ExpandedQuery struct {
	Lex  string `json:"lex"`  // BM25 keyword variant
	Vec  string `json:"vec"`  // Semantic search variant
	Hyde string `json:"hyde"` // Hypothetical document variant
}

// DefaultProvider returns the best available provider.
// Priority: managed llama-server > Ollama.
// The embedOnly flag skips downloading the larger chat model when
// only embedding is needed (e.g., for the embed and vsearch commands).
func DefaultProvider(embedOnly bool) (Provider, error) {
	// If QMD_PROVIDER=ollama is set, use Ollama
	if os.Getenv("QMD_PROVIDER") == "ollama" {
		return NewOllamaProvider(), nil
	}

	// Try managed provider (auto-downloads llama-server + models)
	if embedOnly {
		p, err := NewManagedEmbedOnly()
		if err == nil {
			return p, nil
		}
		fmt.Fprintf(os.Stderr, "managed provider (embed only) failed: %v\n", err)
	} else {
		p, err := NewManagedProvider()
		if err == nil {
			return p, nil
		}
		fmt.Fprintf(os.Stderr, "managed provider failed: %v\n", err)
	}

	// Fall back to Ollama
	fmt.Fprintln(os.Stderr, "Falling back to Ollama provider...")
	return NewOllamaProvider(), nil
}

// CloseProvider closes the provider if it implements Closeable.
func CloseProvider(p Provider) {
	if c, ok := p.(Closeable); ok {
		c.Close()
	}
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
