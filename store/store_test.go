//go:build fts5

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// --- 2.1 Open / Schema / Close ---

func TestOpenClose(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	if s.DB == nil {
		t.Fatal("DB should not be nil")
	}
}

func TestOpenCreatesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "test.sqlite")
	s, err := Open(nested)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat(filepath.Dir(nested)); err != nil {
		t.Errorf("directory should be created: %v", err)
	}
}

func TestSchemaIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	s1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	// Open again — should not error
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

// --- 2.2 Content hashing & title extraction ---

func TestHashContent(t *testing.T) {
	t.Parallel()
	h := HashContent("hello world")
	if len(h) != 64 {
		t.Errorf("hash length = %d, want 64", len(h))
	}
	// Deterministic
	if h != HashContent("hello world") {
		t.Error("hash should be deterministic")
	}
}

func TestHashContentDifferent(t *testing.T) {
	t.Parallel()
	h1 := HashContent("hello")
	h2 := HashContent("world")
	if h1 == h2 {
		t.Error("different bodies should produce different hashes")
	}
}

func TestExtractTitle_H1(t *testing.T) {
	t.Parallel()
	title := ExtractTitle("# My Title\n\nSome content", "file.md")
	if title != "My Title" {
		t.Errorf("title = %q, want 'My Title'", title)
	}
}

func TestExtractTitle_H2(t *testing.T) {
	t.Parallel()
	title := ExtractTitle("## Subtitle\n\nContent", "file.md")
	if title != "Subtitle" {
		t.Errorf("title = %q, want 'Subtitle'", title)
	}
}

func TestExtractTitle_OrgMode(t *testing.T) {
	t.Parallel()
	title := ExtractTitle("#+TITLE: Foo Bar\n\nContent", "file.org")
	if title != "Foo Bar" {
		t.Errorf("title = %q, want 'Foo Bar'", title)
	}
}

func TestExtractTitle_Fallback(t *testing.T) {
	t.Parallel()
	title := ExtractTitle("No heading here\nJust text", "my-notes.md")
	if title != "my-notes" {
		t.Errorf("title = %q, want 'my-notes'", title)
	}
}

func TestExtractTitle_SkipsBlankLines(t *testing.T) {
	t.Parallel()
	title := ExtractTitle("\n\n\n# Real Title\nContent", "file.md")
	if title != "Real Title" {
		t.Errorf("title = %q, want 'Real Title'", title)
	}
}

// --- 2.3 Virtual paths & display paths ---

func TestVirtualPath(t *testing.T) {
	t.Parallel()
	d := &DocumentResult{Collection: "docs", Filepath: "guide/intro.md"}
	vp := d.VirtualPath()
	if vp != "qqmd://docs/guide/intro.md" {
		t.Errorf("VirtualPath() = %q", vp)
	}
}

func TestBuildDisplayPath(t *testing.T) {
	t.Parallel()
	dp := BuildDisplayPath("coll", "file.md")
	if dp != "qqmd://coll/file.md" {
		t.Errorf("BuildDisplayPath = %q", dp)
	}
}

func TestDocid(t *testing.T) {
	t.Parallel()
	d := &DocumentResult{Hash: "abcdef1234567890"}
	if d.Docid() != "abcdef" {
		t.Errorf("Docid() = %q, want 'abcdef'", d.Docid())
	}
	// Short hash returned as-is
	d2 := &DocumentResult{Hash: "abc"}
	if d2.Docid() != "abc" {
		t.Errorf("Docid() = %q, want 'abc'", d2.Docid())
	}
}
