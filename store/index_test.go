//go:build fts5

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/piranha/qqmd/config"
)

func coll(path, pattern string) config.Collection {
	if pattern == "" {
		pattern = "**/*.md"
	}
	return config.Collection{Path: path, Pattern: pattern}
}

// --- 2.4 Indexing ---

func TestIndexCollectionAddsFiles(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"a.md": "# Alpha\n\nContent of alpha.",
		"b.md": "# Beta\n\nContent of beta.",
	})
	stats, err := s.IndexCollection("test", coll(dir, ""))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 2 {
		t.Errorf("added = %d, want 2", stats.Added)
	}
	if stats.Updated != 0 || stats.Removed != 0 {
		t.Errorf("unexpected stats: updated=%d removed=%d", stats.Updated, stats.Removed)
	}
}

func TestIndexCollectionPattern(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"doc.md":  "# Doc",
		"note.txt": "a note",
		"code.go":  "package main",
	})
	stats, err := s.IndexCollection("test", coll(dir, "**/*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 {
		t.Errorf("added = %d, want 1 (only .md)", stats.Added)
	}
}

func TestIndexCollectionSkipsDirs(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"readme.md":              "# Readme",
		".git/config":            "git config",
		"node_modules/pkg/a.md":  "# Package",
	})
	stats, err := s.IndexCollection("test", coll(dir, "**/*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 {
		t.Errorf("added = %d, want 1 (only readme.md)", stats.Added)
	}
}

func TestIndexCollectionSkipsHiddenFiles(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"visible.md": "# Visible",
		".hidden.md": "# Hidden",
	})
	stats, err := s.IndexCollection("test", coll(dir, "**/*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 {
		t.Errorf("added = %d, want 1", stats.Added)
	}
}

func TestIndexCollectionEmpty(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"empty.md": "",
		"blank.md": "   \n  \n  ",
	})
	stats, err := s.IndexCollection("test", coll(dir, "**/*.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Empty body files counted as unchanged
	if stats.Added != 0 {
		t.Errorf("added = %d, want 0 (empty bodies)", stats.Added)
	}
	if stats.Unchanged != 2 {
		t.Errorf("unchanged = %d, want 2", stats.Unchanged)
	}
}

func TestIndexCollectionUpdate(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"doc.md": "# Original\n\nOriginal content.",
	})
	s.IndexCollection("test", coll(dir, ""))

	// Change the file
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Updated\n\nNew content."), 0o644)
	stats, err := s.IndexCollection("test", coll(dir, ""))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Updated != 1 {
		t.Errorf("updated = %d, want 1", stats.Updated)
	}

	// Verify FTS was updated
	results, err := s.SearchFTS("Updated", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("FTS should find updated content")
	}
}

func TestIndexCollectionUnchanged(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"doc.md": "# Doc\n\nContent.",
	})
	s.IndexCollection("test", coll(dir, ""))
	stats, err := s.IndexCollection("test", coll(dir, ""))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Unchanged != 1 {
		t.Errorf("unchanged = %d, want 1", stats.Unchanged)
	}
	if stats.Added != 0 || stats.Updated != 0 {
		t.Error("no adds or updates expected")
	}
}

func TestIndexCollectionRemoved(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"a.md":       "# A",
		"removed.md": "# To Be Removed With Very Unique Name",
	})
	s.IndexCollection("test", coll(dir, ""))

	// Delete one file
	os.Remove(filepath.Join(dir, "removed.md"))
	stats, err := s.IndexCollection("test", coll(dir, ""))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Removed != 1 {
		t.Errorf("removed = %d, want 1", stats.Removed)
	}

	// Verify the document is marked inactive in the database
	var active int
	err = s.DB.QueryRow("SELECT active FROM documents WHERE collection='test' AND filepath='removed.md'").Scan(&active)
	if err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Error("document should be inactive after removal")
	}

	// Verify it doesn't appear in ListFiles
	files, _ := s.ListFiles("test")
	for _, f := range files {
		if f.Filepath == "removed.md" {
			t.Error("removed file should not appear in ListFiles")
		}
	}
}

func TestDeactivateCollection(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := createTestFiles(t, map[string]string{
		"a.md": "# A\n\nContent A.",
		"b.md": "# B\n\nContent B.",
	})
	s.IndexCollection("test", coll(dir, ""))

	if err := s.DeactivateCollection("test"); err != nil {
		t.Fatal(err)
	}

	files, _ := s.ListFiles("")
	if len(files) != 0 {
		t.Errorf("expected 0 active files, got %d", len(files))
	}

	// FTS should not return results
	results, _ := s.SearchFTS("Content", SearchOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("expected 0 FTS results after deactivation, got %d", len(results))
	}
}

func TestIndexCollectionBadPath(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	_, err := s.IndexCollection("test", coll("/nonexistent/path/12345", ""))
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestIndexCollectionNotADir(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("hello"), 0o644)
	_, err := s.IndexCollection("test", coll(file, ""))
	if err == nil {
		t.Error("expected error for file path (not dir)")
	}
}
