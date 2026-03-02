// Package llm provides interfaces for LLM operations (embeddings, reranking, query expansion)
// with pluggable backends: managed llama-server, Ollama, and OpenAI.
package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/piranha/qqmd/config"
)

// Embedder is the interface for embedding backends.
type Embedder interface {
	// Embed returns a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch returns embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedDimension returns the embedding vector dimension, or 0 if unknown.
	EmbedDimension() int

	// Name returns the backend name for display/storage.
	Name() string
}

// ChatProvider is the interface for chat/generation backends (reranking, query expansion).
type ChatProvider interface {
	// Rerank scores documents against a query. Returns scores in the same order as docs.
	Rerank(ctx context.Context, query string, docs []string) ([]float64, error)

	// ExpandQuery generates expanded query variants for hybrid search.
	ExpandQuery(ctx context.Context, query string) (*ExpandedQuery, error)

	// Name returns the backend name for display.
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

// DefaultEmbedder returns the best available embedding backend.
// Settings are read from the config file and can be overridden by environment variables.
// Priority: explicit backend (openai, ollama) > managed llama-server > Ollama.
func DefaultEmbedder() (Embedder, error) {
	ec := config.GetEmbedConfig()

	switch ec.Backend {
	case "openai":
		return NewOpenAIEmbedder(ec.APIKey, ec.BaseURL, ec.Model)
	case "ollama":
		return NewOllamaEmbedder(ec.BaseURL, ec.Model), nil
	case "":
		// Default: try managed, fall back to Ollama
	default:
		return nil, fmt.Errorf("unknown embed backend %q (supported: openai, ollama)", ec.Backend)
	}

	// Legacy env override
	if os.Getenv("QQMD_PROVIDER") == "ollama" {
		return NewOllamaEmbedder(ec.BaseURL, ec.Model), nil
	}

	// Try managed provider
	p, err := NewManagedEmbedOnly()
	if err == nil {
		return p, nil
	}
	fmt.Fprintf(os.Stderr, "managed embedder failed: %v\n", err)

	// Fall back to Ollama
	fmt.Fprintln(os.Stderr, "Falling back to Ollama embedder...")
	return NewOllamaEmbedder(ec.BaseURL, ec.Model), nil
}

// DefaultChatProvider returns the best available chat provider.
// When the embed backend is explicitly set to a remote provider (e.g. openai),
// chat features are skipped unless QQMD_PROVIDER is explicitly set.
// This avoids downloading local models when the user wants a remote-only setup.
func DefaultChatProvider() (ChatProvider, error) {
	ec := config.GetEmbedConfig()

	if os.Getenv("QQMD_PROVIDER") == "ollama" {
		return NewOllamaChatProvider(), nil
	}

	// If user explicitly configured a remote embed backend and didn't
	// also set QQMD_PROVIDER, don't try to download local models for chat.
	if ec.Backend != "" && ec.Backend != "ollama" {
		return nil, fmt.Errorf("chat provider not configured (skipping reranking/expansion)")
	}

	p, err := NewManagedProvider()
	if err == nil {
		return p, nil
	}
	fmt.Fprintf(os.Stderr, "managed chat provider failed: %v\n", err)

	fmt.Fprintln(os.Stderr, "Falling back to Ollama chat provider...")
	return NewOllamaChatProvider(), nil
}

// CloseIfNeeded closes the provider if it implements Closeable.
func CloseIfNeeded(v any) {
	if c, ok := v.(Closeable); ok {
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
