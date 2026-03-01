package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/piranha/qqmd/config"
)

// IndexStats holds counts from an indexing operation.
type IndexStats struct {
	Added     int
	Updated   int
	Unchanged int
	Removed   int
}

// Directories to skip during indexing.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".cache":       true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
}

// IndexCollection walks the collection's directory and indexes all matching files.
func (s *Store) IndexCollection(name string, coll config.Collection) (*IndexStats, error) {
	stats := &IndexStats{}
	root := coll.Path

	// Expand ~ in path
	if strings.HasPrefix(root, "~/") {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, root[2:])
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("collection path %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("collection path %q is not a directory", root)
	}

	pattern := coll.Pattern
	if pattern == "" {
		pattern = "**/*.md"
	}

	// Track which files we've seen so we can deactivate removed ones
	seen := make(map[string]bool)

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || skipDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}
		// Check if hidden file (any path component starts with .)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		for _, p := range parts {
			if strings.HasPrefix(p, ".") {
				return nil
			}
		}
		// Check glob pattern match
		matched, err := doublestar.Match(pattern, rel)
		if err != nil || !matched {
			return nil
		}

		seen[rel] = true
		action, err := s.indexFile(name, rel, path)
		if err != nil {
			return fmt.Errorf("indexing %s: %w", rel, err)
		}
		switch action {
		case "added":
			stats.Added++
		case "updated":
			stats.Updated++
		case "unchanged":
			stats.Unchanged++
		}
		return nil
	})
	if err != nil {
		return stats, err
	}

	// Deactivate documents that no longer exist on disk
	removed, err := s.deactivateMissing(name, seen)
	if err != nil {
		return stats, fmt.Errorf("deactivating removed docs: %w", err)
	}
	stats.Removed = removed

	return stats, nil
}

func (s *Store) indexFile(collection, relPath, absPath string) (string, error) {
	bodyBytes, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	body := string(bodyBytes)
	if strings.TrimSpace(body) == "" {
		return "unchanged", nil
	}

	hash := HashContent(body)
	title := ExtractTitle(body, relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	modTime := info.ModTime().UTC().Format(time.RFC3339)

	// Check if document already exists
	var existingID int64
	var existingHash string
	err = s.DB.QueryRow(
		"SELECT id, hash FROM documents WHERE collection=? AND filepath=? AND active=1",
		collection, relPath,
	).Scan(&existingID, &existingHash)

	if err == sql.ErrNoRows {
		// New document
		if _, err := s.DB.Exec(
			"INSERT OR IGNORE INTO content (hash, body) VALUES (?, ?)",
			hash, body,
		); err != nil {
			return "", err
		}
		result, err := s.DB.Exec(
			`INSERT INTO documents (collection, filepath, title, hash, modified_at, body_length, active)
			 VALUES (?, ?, ?, ?, ?, ?, 1)`,
			collection, relPath, title, hash, modTime, len(body),
		)
		if err != nil {
			return "", err
		}
		docID, _ := result.LastInsertId()
		if _, err := s.DB.Exec(
			"INSERT INTO documents_fts (rowid, filepath, title, body) VALUES (?, ?, ?, ?)",
			docID, collection+"/"+relPath, title, body,
		); err != nil {
			return "", err
		}
		return "added", nil
	}
	if err != nil {
		return "", err
	}

	// Exists — check if content changed
	if existingHash == hash {
		return "unchanged", nil
	}

	// Content changed — update
	if _, err := s.DB.Exec(
		"INSERT OR IGNORE INTO content (hash, body) VALUES (?, ?)",
		hash, body,
	); err != nil {
		return "", err
	}

	// Get old body for FTS delete
	var oldBody, oldTitle string
	s.DB.QueryRow("SELECT c.body, d.title FROM documents d JOIN content c ON d.hash = c.hash WHERE d.id = ?",
		existingID).Scan(&oldBody, &oldTitle)

	if _, err := s.DB.Exec(
		"UPDATE documents SET title=?, hash=?, modified_at=?, body_length=? WHERE id=?",
		title, hash, modTime, len(body), existingID,
	); err != nil {
		return "", err
	}

	// Update FTS: delete old, insert new
	if _, err := s.DB.Exec(
		"INSERT INTO documents_fts(documents_fts, rowid, filepath, title, body) VALUES('delete', ?, ?, ?, ?)",
		existingID, collection+"/"+relPath, oldTitle, oldBody,
	); err != nil {
		// FTS delete may fail if content doesn't match exactly; just proceed
		_ = err
	}
	if _, err := s.DB.Exec(
		"INSERT INTO documents_fts (rowid, filepath, title, body) VALUES (?, ?, ?, ?)",
		existingID, collection+"/"+relPath, title, body,
	); err != nil {
		return "", err
	}

	return "updated", nil
}

func (s *Store) deactivateMissing(collection string, seen map[string]bool) (int, error) {
	rows, err := s.DB.Query(
		"SELECT id, filepath, title, hash FROM documents WHERE collection=? AND active=1",
		collection,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var toRemove []struct {
		id       int64
		filepath string
		title    string
		hash     string
	}
	for rows.Next() {
		var r struct {
			id       int64
			filepath string
			title    string
			hash     string
		}
		if err := rows.Scan(&r.id, &r.filepath, &r.title, &r.hash); err != nil {
			return 0, err
		}
		if !seen[r.filepath] {
			toRemove = append(toRemove, r)
		}
	}

	for _, r := range toRemove {
		if _, err := s.DB.Exec("UPDATE documents SET active=0 WHERE id=?", r.id); err != nil {
			return 0, err
		}
		// Remove from FTS
		var body string
		s.DB.QueryRow("SELECT body FROM content WHERE hash=?", r.hash).Scan(&body)
		s.DB.Exec(
			"INSERT INTO documents_fts(documents_fts, rowid, filepath, title, body) VALUES('delete', ?, ?, ?, ?)",
			r.id, collection+"/"+r.filepath, r.title, body,
		)
	}

	return len(toRemove), nil
}

// DeactivateCollection marks all documents in a collection as inactive and removes FTS entries.
func (s *Store) DeactivateCollection(collection string) error {
	rows, err := s.DB.Query(
		"SELECT id, filepath, title, hash FROM documents WHERE collection=? AND active=1",
		collection,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type doc struct {
		id       int64
		filepath string
		title    string
		hash     string
	}
	var docs []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.id, &d.filepath, &d.title, &d.hash); err != nil {
			return err
		}
		docs = append(docs, d)
	}

	for _, d := range docs {
		if _, err := s.DB.Exec("UPDATE documents SET active=0 WHERE id=?", d.id); err != nil {
			return err
		}
		var body string
		s.DB.QueryRow("SELECT body FROM content WHERE hash=?", d.hash).Scan(&body)
		s.DB.Exec(
			"INSERT INTO documents_fts(documents_fts, rowid, filepath, title, body) VALUES('delete', ?, ?, ?, ?)",
			d.id, collection+"/"+d.filepath, d.title, body,
		)
	}
	return nil
}
