//go:build fts5

package store

import (
	"math"
	"strings"
	"testing"
)

// --- 2.6 Search ---

func TestSearchFTS_Basic(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "guide.md", "# Installation Guide\n\nHow to install the software.")
	mustInsertDoc(t, s, "docs", "faq.md", "# FAQ\n\nFrequently asked questions.")

	results, err := s.SearchFTS("install", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result")
	}
	if results[0].Filepath != "guide.md" {
		t.Errorf("expected guide.md first, got %q", results[0].Filepath)
	}
}

func TestSearchFTS_NoResults(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "a.md", "# About Cats\n\nCats are great.")

	results, err := s.SearchFTS("dinosaur", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchFTS_CollectionFilter(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "alpha", "a.md", "# Search Term Alpha")
	mustInsertDoc(t, s, "beta", "b.md", "# Search Term Beta")

	results, err := s.SearchFTS("Search Term", SearchOptions{
		Collections: []string{"alpha"},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Collection != "alpha" {
		t.Errorf("collection = %q, want alpha", results[0].Collection)
	}
}

func TestSearchFTS_MinScore(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "exact.md", "# Elasticsearch Guide\n\nElasticsearch is a search engine.")
	mustInsertDoc(t, s, "test", "vague.md", "# Cooking Recipes\n\nSearch for your favorite recipe.")

	results, err := s.SearchFTS("elasticsearch guide", SearchOptions{
		Limit:    10,
		MinScore: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Score < 0.9 {
			t.Errorf("result %q has score %.2f below min 0.9", r.Filepath, r.Score)
		}
	}
}

func TestSearchFTS_Limit(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	for i := 0; i < 10; i++ {
		mustInsertDoc(t, s, "test", "doc"+string(rune('a'+i))+".md", "# Document about search engines\n\nSearch content.")
	}

	results, err := s.SearchFTS("search", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 3 {
		t.Errorf("expected max 3 results, got %d", len(results))
	}
}

func TestSearchFTS_ScoreNormalization(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "a.md", "# Important Topic\n\nThe important topic discussed here.")
	mustInsertDoc(t, s, "test", "b.md", "# Another Doc\n\nSomething somewhat important.")

	results, err := s.SearchFTS("important topic", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Top result should have score 1.0 (normalized)
	if math.Abs(results[0].Score-1.0) > 0.01 {
		t.Errorf("top score = %.2f, want 1.0", results[0].Score)
	}
}

func TestBuildFTS5Query_Passthrough(t *testing.T) {
	t.Parallel()
	cases := []string{
		`"exact phrase"`,
		`foo OR bar`,
		`foo AND bar`,
		`NEAR(foo bar, 5)`,
	}
	for _, q := range cases {
		got := buildFTS5Query(q)
		if got != q {
			t.Errorf("buildFTS5Query(%q) = %q, want passthrough", q, got)
		}
	}
}

func TestBuildFTS5Query_SingleWord(t *testing.T) {
	t.Parallel()
	got := buildFTS5Query("hello")
	if got != `"hello"` {
		t.Errorf("got %q, want exact phrase wrapping", got)
	}
}

func TestBuildFTS5Query_MultiWord(t *testing.T) {
	t.Parallel()
	got := buildFTS5Query("foo bar")
	// Should contain exact phrase, NEAR, and OR terms
	if got == "" {
		t.Fatal("empty result")
	}
	if !containsAll(got, `"foo bar"`, "NEAR", "OR") {
		t.Errorf("got %q, expected exact + NEAR + OR", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// --- 2.6 Vec Search ---

func TestSearchVec_Basic(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "# Vector Doc\n\nThis document is about vectors."
	mustInsertDoc(t, s, "test", "vec.md", body)
	hash := HashContent(body)

	// Store an embedding
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	s.StoreEmbedding(hash, 0, 0, len(body), emb, "test")

	// Query with similar vector
	query := []float32{0.9, 0.1, 0.0, 0.0}
	results, err := s.SearchVec(query, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result")
	}
	if results[0].Filepath != "vec.md" {
		t.Errorf("filepath = %q", results[0].Filepath)
	}
}

func TestSearchVec_Dedup(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "# Multi Chunk\n\nLong document with multiple chunks."
	mustInsertDoc(t, s, "test", "multi.md", body)
	hash := HashContent(body)

	// Store multiple chunk embeddings for same doc
	s.StoreEmbedding(hash, 0, 0, 20, []float32{1.0, 0.0, 0.0}, "test")
	s.StoreEmbedding(hash, 1, 20, 40, []float32{0.5, 0.5, 0.0}, "test")

	query := []float32{1.0, 0.0, 0.0}
	results, err := s.SearchVec(query, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	// Should only get one result per document
	if len(results) != 1 {
		t.Errorf("expected 1 result (deduped), got %d", len(results))
	}
}

func TestHybridSearch_MergesFTSAndVec(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)

	fts := []SearchResult{
		{DocumentResult: DocumentResult{ID: 1, Filepath: "a.md"}, Score: 1.0, Source: "fts"},
		{DocumentResult: DocumentResult{ID: 2, Filepath: "b.md"}, Score: 0.5, Source: "fts"},
	}
	vec := []SearchResult{
		{DocumentResult: DocumentResult{ID: 2, Filepath: "b.md"}, Score: 0.9, Source: "vec"},
		{DocumentResult: DocumentResult{ID: 3, Filepath: "c.md"}, Score: 0.7, Source: "vec"},
	}

	results := s.HybridSearch(fts, vec, 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 merged results, got %d", len(results))
	}
	// Top result should have score 1.0 (normalized)
	if math.Abs(results[0].Score-1.0) > 0.01 {
		t.Errorf("top score = %.4f, want 1.0", results[0].Score)
	}
}

func TestHybridSearch_Empty(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	results := s.HybridSearch(nil, nil, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestEffectiveLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts SearchOptions
		want int
	}{
		{"default", SearchOptions{}, 5},
		{"explicit", SearchOptions{Limit: 20}, 20},
		{"showAll", SearchOptions{ShowAll: true}, 10000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.opts.EffectiveLimit()
			if got != tc.want {
				t.Errorf("EffectiveLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- 2.7 Embeddings ---

func TestStoreAndRetrieveEmbedding(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "# Test\n\nEmbedding test doc."
	mustInsertDoc(t, s, "test", "emb.md", body)
	hash := HashContent(body)

	emb := []float32{0.1, 0.2, 0.3, 0.4}
	if err := s.StoreEmbedding(hash, 0, 0, len(body), emb, "model-x"); err != nil {
		t.Fatal(err)
	}

	// Retrieve via SearchVec
	query := []float32{0.1, 0.2, 0.3, 0.4}
	results, err := s.SearchVec(query, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected results from stored embedding")
	}
}

func TestHasEmbeddings(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	hash := HashContent("test content")
	if s.HasEmbeddings(hash) {
		t.Error("should be false before storing")
	}
	s.StoreEmbedding(hash, 0, 0, 10, []float32{1, 2, 3}, "model")
	if !s.HasEmbeddings(hash) {
		t.Error("should be true after storing")
	}
}

func TestDeleteEmbeddings(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	hash := HashContent("test content")
	s.StoreEmbedding(hash, 0, 0, 10, []float32{1, 2, 3}, "model")
	if err := s.DeleteEmbeddings(hash); err != nil {
		t.Fatal(err)
	}
	if s.HasEmbeddings(hash) {
		t.Error("embeddings should be gone after delete")
	}
}

func TestEmbeddingStats(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "a.md", "# A unique body")
	mustInsertDoc(t, s, "test", "b.md", "# B unique body")
	hashA := HashContent("# A unique body")
	hashB := HashContent("# B unique body")

	s.StoreEmbedding(hashA, 0, 0, 10, []float32{1, 2}, "model")
	s.StoreEmbedding(hashB, 0, 0, 10, []float32{3, 4}, "model")

	embedded, total, err := s.EmbeddingStats()
	if err != nil {
		t.Fatal(err)
	}
	if embedded != 2 {
		t.Errorf("embedded = %d, want 2", embedded)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	t.Parallel()
	original := []float32{1.5, -2.3, 0.0, 3.14159}
	encoded := encodeEmbedding(original)
	decoded := decodeEmbedding(encoded)
	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("index %d: got %f, want %f", i, decoded[i], original[i])
		}
	}
}

// --- 2.10 Helpers (continued) ---

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()
	// Identical vectors → 1.0
	v := []float32{1, 2, 3}
	sim := cosineSimilarity(v, v)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical = %.6f, want 1.0", sim)
	}

	// Orthogonal → 0.0
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim = cosineSimilarity(a, b)
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal = %.6f, want 0.0", sim)
	}

	// Opposite → -1.0
	c := []float32{1, 0, 0}
	d := []float32{-1, 0, 0}
	sim = cosineSimilarity(c, d)
	if math.Abs(sim+1.0) > 1e-6 {
		t.Errorf("opposite = %.6f, want -1.0", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	t.Parallel()
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different lengths = %.6f, want 0", sim)
	}
}

func TestExtractSnippet(t *testing.T) {
	t.Parallel()
	body := "Line one about cats.\nLine two about dogs.\nLine three about search engines.\nLine four about databases.\nLine five about nothing."
	snippet, line := ExtractSnippet(body, "search engines", 300)
	if snippet == "" {
		t.Error("expected non-empty snippet")
	}
	if line == 0 {
		t.Error("expected non-zero line")
	}
	if len(snippet) > 300+3 { // +3 for "..."
		t.Errorf("snippet too long: %d", len(snippet))
	}
}

func TestExtractSnippet_EmptyBody(t *testing.T) {
	t.Parallel()
	snippet, line := ExtractSnippet("", "query", 300)
	if snippet != "" {
		t.Errorf("snippet = %q, want empty", snippet)
	}
	if line != 0 {
		t.Errorf("line = %d, want 0", line)
	}
}

// --- Edge cases ported from qmd ---

func TestSearchFTS_UnicodeContent(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "unicode.md",
		"# Ünïcödë Tïtlé\n\nCafé résumé naïve über coöperate.\n\nEmoji: 🎉🚀✨")

	results, err := s.SearchFTS("résumé", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected to find unicode content")
	}

	// Verify retrieval preserves unicode
	body, err := s.GetContentBody(results[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "🎉") {
		t.Error("body should contain emoji")
	}
}

func TestSearchFTS_VeryLongDocument(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	longBody := "# Long Doc\n\n"
	for i := 0; i < 10000; i++ {
		longBody += "searchable word in a very long document. "
	}
	mustInsertDoc(t, s, "test", "long.md", longBody)

	results, err := s.SearchFTS("searchable", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestSearchFTS_EmptyDatabase(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	results, err := s.SearchFTS("anything", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty db, got %d", len(results))
	}
}

func TestSearchFTS_MultipleCollections(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "coll1", "a.md", "# Searchable content one")
	mustInsertDoc(t, s, "coll2", "b.md", "# Searchable content two")
	mustInsertDoc(t, s, "coll3", "c.md", "# Searchable content three")

	// Search across specific collections
	results, err := s.SearchFTS("searchable content", SearchOptions{
		Collections: []string{"coll1", "coll3"},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Collection != "coll1" && r.Collection != "coll3" {
			t.Errorf("unexpected collection %q", r.Collection)
		}
	}
}

func TestSearchFTS_TitleMatchRanksHigher(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "body.md", "# Other Title\n\nSomething about kubernetes.")
	mustInsertDoc(t, s, "test", "title.md", "# Kubernetes Guide\n\nDifferent content about kubernetes.")

	results, err := s.SearchFTS("kubernetes", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Title match should rank higher due to BM25 weights
	if results[0].Filepath != "title.md" {
		t.Errorf("expected title.md first, got %q", results[0].Filepath)
	}
}
