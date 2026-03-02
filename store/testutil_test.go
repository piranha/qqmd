//go:build fts5

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// mustOpenMemory opens an in-memory store for testing.
func mustOpenMemory(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mustOpenDir opens a file-backed store in a temp directory.
func mustOpenDir(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mustInsertDoc inserts a document into the store and returns its ID.
func mustInsertDoc(t *testing.T, s *Store, collection, relPath, body string) int64 {
	t.Helper()
	hash := HashContent(body)
	title := ExtractTitle(body, relPath)

	if _, err := s.DB.Exec(
		"INSERT OR IGNORE INTO content (hash, body) VALUES (?, ?)",
		hash, body,
	); err != nil {
		t.Fatal(err)
	}

	result, err := s.DB.Exec(
		`INSERT INTO documents (collection, filepath, title, hash, modified_at, body_length, active)
		 VALUES (?, ?, ?, ?, datetime('now'), ?, 1)`,
		collection, relPath, title, hash, len(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	docID, _ := result.LastInsertId()

	if _, err := s.DB.Exec(
		"INSERT INTO documents_fts (rowid, filepath, title, body) VALUES (?, ?, ?, ?)",
		docID, collection+"/"+relPath, title, body,
	); err != nil {
		t.Fatal(err)
	}
	return docID
}

// createTestFiles creates markdown files in a temp directory for indexing tests.
func createTestFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
