//go:build fts5

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- 2.5 Document lookup ---

func TestFindByVirtualPath(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "guide/intro.md", "# Introduction\n\nWelcome.")

	doc, err := s.FindDocument("qqmd://docs/guide/intro.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filepath != "guide/intro.md" {
		t.Errorf("filepath = %q", doc.Filepath)
	}
	if doc.Collection != "docs" {
		t.Errorf("collection = %q", doc.Collection)
	}
}

func TestFindByVirtualPathInvalid(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	_, err := s.FindDocument("qqmd://docs")
	if err == nil {
		t.Error("expected error for invalid virtual path (no file part)")
	}
}

func TestFindByDocid(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "coll", "file.md", "# Test Doc\n\nBody text here.")
	hash := HashContent("# Test Doc\n\nBody text here.")
	prefix := hash[:6]

	doc, err := s.FindDocument("#" + prefix)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filepath != "file.md" {
		t.Errorf("filepath = %q", doc.Filepath)
	}
}

func TestFindByDocidNotFound(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "coll", "file.md", "content")
	_, err := s.FindDocument("#zzzzzz")
	if err == nil {
		t.Error("expected error for non-matching prefix")
	}
}

func TestFindByCollectionPath(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "wiki", "api/auth.md", "# Auth API")

	doc, err := s.FindDocument("wiki/api/auth.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filepath != "api/auth.md" {
		t.Errorf("filepath = %q", doc.Filepath)
	}
}

func TestFindByFilepath(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "alpha", "readme.md", "# Readme\n\nAlpha readme.")
	mustInsertDoc(t, s, "beta", "notes.md", "# Notes\n\nBeta notes.")

	doc, err := s.FindDocument("readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Collection != "alpha" {
		t.Errorf("collection = %q, want 'alpha'", doc.Collection)
	}
}

func TestFindBySuffix(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "guides/deep/nested.md", "# Nested")

	doc, err := s.FindDocument("deep/nested.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filepath != "guides/deep/nested.md" {
		t.Errorf("filepath = %q", doc.Filepath)
	}
}

func TestFindFuzzy(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "introduction.md", "# Intro")

	// Typo within distance 3
	doc, err := s.FindDocument("docs/introductoin.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filepath != "introduction.md" {
		t.Errorf("filepath = %q, want introduction.md", doc.Filepath)
	}
}

func TestFindFuzzyTooFar(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "docs", "introduction.md", "# Intro")

	// Typo > distance 3
	_, err := s.FindDocument("docs/xxxxxxxxxxxxxx.md")
	if err == nil {
		t.Error("expected error for typo too far")
	}
}

func TestGetDocumentBodyFull(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	mustInsertDoc(t, s, "test", "file.md", body)
	doc, _ := s.FindDocument("qqmd://test/file.md")

	got, err := s.GetDocumentBody(doc, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Errorf("body mismatch:\ngot:  %q\nwant: %q", got, body)
	}
}

func TestGetDocumentBodyLineRange(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	mustInsertDoc(t, s, "test", "file.md", body)
	doc, _ := s.FindDocument("qqmd://test/file.md")

	got, err := s.GetDocumentBody(doc, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Line 3\nLine 4" {
		t.Errorf("body = %q, want 'Line 3\\nLine 4'", got)
	}
}

func TestGetDocumentBodyPastEnd(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "Line 1\nLine 2"
	mustInsertDoc(t, s, "test", "file.md", body)
	doc, _ := s.FindDocument("qqmd://test/file.md")

	got, err := s.GetDocumentBody(doc, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("body = %q, want empty", got)
	}
}

func TestFindDocumentsGlob(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "guide/a.md", "# A")
	mustInsertDoc(t, s, "test", "guide/b.md", "# B")
	mustInsertDoc(t, s, "test", "notes.txt", "notes")

	docs, err := s.FindDocuments("**/*.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Errorf("found %d docs, want 2", len(docs))
	}
}

func TestFindDocumentsCommaList(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "a.md", "# A")
	mustInsertDoc(t, s, "test", "b.md", "# B")
	mustInsertDoc(t, s, "test", "c.md", "# C")

	docs, err := s.FindDocuments("a.md,b.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Errorf("found %d docs, want 2", len(docs))
	}
}

func TestListFiles(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "a", "f1.md", "# F1")
	mustInsertDoc(t, s, "b", "f2.md", "# F2")

	files, err := s.ListFiles("")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("files = %d, want 2", len(files))
	}
}

func TestListFilesFiltered(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "alpha", "a.md", "# A")
	mustInsertDoc(t, s, "beta", "b.md", "# B")

	files, err := s.ListFiles("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("files = %d, want 1", len(files))
	}
	if files[0].Collection != "alpha" {
		t.Errorf("collection = %q", files[0].Collection)
	}
}

// --- 2.9 Cleanup & status ---

func TestCleanup_RemovesInactive(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "active.md", "# Active")
	id := mustInsertDoc(t, s, "test", "inactive.md", "# Inactive")
	s.DB.Exec("UPDATE documents SET active=0 WHERE id=?", id)

	_, docsRemoved, err := s.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if docsRemoved != 1 {
		t.Errorf("docsRemoved = %d, want 1", docsRemoved)
	}
}

func TestCleanup_KeepsActive(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "keep.md", "# Keep me\n\nImportant content.")
	_, docsRemoved, err := s.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if docsRemoved != 0 {
		t.Errorf("docsRemoved = %d, want 0", docsRemoved)
	}
	// Verify content still accessible
	doc, _ := s.FindDocument("qqmd://test/keep.md")
	if doc == nil {
		t.Error("active doc should still be findable")
	}
}

func TestCleanup_RemovesOrphanedEmbeddings(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "# Embedded doc\n\nSome text."
	id := mustInsertDoc(t, s, "test", "emb.md", body)
	hash := HashContent(body)
	s.StoreEmbedding(hash, 0, 0, len(body), []float32{1, 2, 3}, "test-model")

	// Deactivate
	s.DB.Exec("UPDATE documents SET active=0 WHERE id=?", id)
	s.Cleanup()

	if s.HasEmbeddings(hash) {
		t.Error("embeddings should be removed after cleanup")
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "alpha", "a.md", "# A")
	mustInsertDoc(t, s, "alpha", "b.md", "# B")
	mustInsertDoc(t, s, "beta", "c.md", "# C")

	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalDocuments != 3 {
		t.Errorf("TotalDocuments = %d, want 3", st.TotalDocuments)
	}
	if st.Collections["alpha"] != 2 {
		t.Errorf("alpha count = %d, want 2", st.Collections["alpha"])
	}
	if st.Collections["beta"] != 1 {
		t.Errorf("beta count = %d, want 1", st.Collections["beta"])
	}
}

func TestGetAllActiveHashes(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "a.md", "# A unique content")
	mustInsertDoc(t, s, "test", "b.md", "# B different content")
	id := mustInsertDoc(t, s, "test", "c.md", "# C will be deactivated")
	s.DB.Exec("UPDATE documents SET active=0 WHERE id=?", id)

	hashes, err := s.GetAllActiveHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 {
		t.Errorf("got %d hashes, want 2", len(hashes))
	}
}

// --- 2.10 Helpers ---

func TestAddLineNumbers(t *testing.T) {
	t.Parallel()
	text := "hello\nworld\nfoo"
	got := AddLineNumbers(text, 1)
	if !strings.Contains(got, "1: hello") {
		t.Errorf("missing line 1, got:\n%s", got)
	}
	if !strings.Contains(got, "3: foo") {
		t.Errorf("missing line 3, got:\n%s", got)
	}
}

func TestAddLineNumbers_StartOffset(t *testing.T) {
	t.Parallel()
	text := "a\nb\nc"
	got := AddLineNumbers(text, 10)
	if !strings.Contains(got, "10: a") {
		t.Errorf("expected start at 10, got:\n%s", got)
	}
	if !strings.Contains(got, "12: c") {
		t.Errorf("expected line 12, got:\n%s", got)
	}
}

// --- Content-addressable storage tests ported from qmd ---

func TestContentDedup_SameHashAcrossCollections(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	body := "# Shared Content\n\nThis is the same content."
	mustInsertDoc(t, s, "coll1", "shared.md", body)
	mustInsertDoc(t, s, "coll2", "shared.md", body)

	// Both documents should reference the same hash
	hash := HashContent(body)
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM content WHERE hash=?", hash).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 content entry, got %d", count)
	}

	var docCount int
	s.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE hash=?", hash).Scan(&docCount)
	if docCount != 2 {
		t.Errorf("expected 2 documents pointing to same hash, got %d", docCount)
	}
}

func TestContentDedup_DifferentContentDifferentHash(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "a.md", "# Content One")
	mustInsertDoc(t, s, "test", "b.md", "# Content Two")

	hashA := HashContent("# Content One")
	hashB := HashContent("# Content Two")
	if hashA == hashB {
		t.Error("different content should have different hashes")
	}

	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM content").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 content entries, got %d", count)
	}
}

func TestDocumentReactivation(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"doc.md": "# First Version\n\nOriginal content.",
	})

	// Index it
	s.IndexCollection("test", coll(dir, ""))

	// Verify it's active
	files, _ := s.ListFiles("")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	// Remove the file and re-index to deactivate
	os.Remove(filepath.Join(dir, "doc.md"))
	s.IndexCollection("test", coll(dir, ""))
	files, _ = s.ListFiles("")
	if len(files) != 0 {
		t.Fatalf("expected 0 files after removal, got %d", len(files))
	}

	// Bring the file back with new content
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Second Version\n\nNew content."), 0o644)
	stats, err := s.IndexCollection("test", coll(dir, ""))
	if err != nil {
		t.Fatal(err)
	}

	// Should be added back (reactivated)
	files, _ = s.ListFiles("")
	if len(files) != 1 {
		t.Fatalf("expected 1 file after reactivation, got %d", len(files))
	}

	// Should only have one document row (not duplicate)
	var rowCount int
	s.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE collection='test' AND filepath='doc.md'").Scan(&rowCount)
	if rowCount != 1 {
		t.Errorf("expected 1 document row, got %d (duplicate?)", rowCount)
	}

	// New content should be searchable
	results, _ := s.SearchFTS("Second Version", SearchOptions{Limit: 10})
	if len(results) == 0 {
		t.Error("new content should be searchable after reactivation")
	}

	_ = stats
}

func TestDocumentSpecialCharsInPath(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	mustInsertDoc(t, s, "test", "file with spaces.md", "# Spaces In Path\n\nContent.")

	doc, err := s.FindDocument("test/file with spaces.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Error("should find document with spaces in path")
	}
}

func TestLevenshtein(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			got := levenshtein(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
