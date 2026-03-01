// Package store provides SQLite-backed document storage with FTS5 full-text search
// and vector similarity search for qqmd.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	DB     *sql.DB
	DBPath string
}

type DocumentResult struct {
	ID          int64
	Collection  string
	Filepath    string
	DisplayPath string
	Title       string
	Hash        string
	ModifiedAt  string
	BodyLength  int
	Body        string
	Context     string
}

type SearchResult struct {
	DocumentResult
	Score    float64
	Source   string // "fts" or "vec"
	ChunkPos int
}

type Chunk struct {
	Text  string
	Start int
	End   int
}

func (d *DocumentResult) Docid() string {
	if len(d.Hash) >= 6 {
		return d.Hash[:6]
	}
	return d.Hash
}

func (d *DocumentResult) VirtualPath() string {
	return "qmd://" + d.Collection + "/" + d.Filepath
}

func DefaultDBPath() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "qmd", "index.sqlite")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "qmd", "index.sqlite")
}

func Open(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Enable WAL mode and foreign keys
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting %s: %w", pragma, err)
		}
	}
	s := &Store{DB: db, DBPath: dbPath}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) initSchema() error {
	for _, stmt := range schemaStatements {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("executing schema: %s: %w", stmt[:min(60, len(stmt))], err)
		}
	}
	return nil
}

func HashContent(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

func ExtractTitle(body, filepath string) string {
	lines := strings.SplitN(body, "\n", 20)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Markdown heading
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		// Org-mode title
		if strings.HasPrefix(strings.ToLower(line), "#+title:") {
			return strings.TrimSpace(line[8:])
		}
	}
	// Fall back to filename without extension
	base := strings.TrimSuffix(filepath, ".md")
	base = strings.TrimSuffix(base, ".txt")
	base = strings.TrimSuffix(base, ".org")
	parts := strings.Split(base, "/")
	return parts[len(parts)-1]
}

// BuildDisplayPath returns collection/filepath as display path.
func BuildDisplayPath(collection, fp string) string {
	return "qmd://" + collection + "/" + fp
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS content (
		hash TEXT PRIMARY KEY,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		collection TEXT NOT NULL,
		filepath TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		hash TEXT NOT NULL,
		modified_at TEXT NOT NULL,
		body_length INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1,
		UNIQUE(collection, filepath)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_documents_collection ON documents(collection, active)`,
	`CREATE INDEX IF NOT EXISTS idx_documents_hash ON documents(hash)`,
	`CREATE INDEX IF NOT EXISTS idx_documents_path ON documents(filepath, active)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
		filepath, title, body,
		tokenize='porter unicode61'
	)`,
	`CREATE TABLE IF NOT EXISTS embeddings (
		hash TEXT NOT NULL,
		seq INTEGER NOT NULL DEFAULT 0,
		chunk_start INTEGER NOT NULL DEFAULT 0,
		chunk_end INTEGER NOT NULL DEFAULT 0,
		embedding BLOB,
		model TEXT NOT NULL DEFAULT '',
		embedded_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY(hash, seq)
	)`,
	`CREATE TABLE IF NOT EXISTS llm_cache (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
}
