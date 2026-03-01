package store

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// FindDocument looks up a document by path, virtual path, or docid (#hash prefix).
func (s *Store) FindDocument(ref string) (*DocumentResult, error) {
	// Try docid (#hash)
	if strings.HasPrefix(ref, "#") {
		return s.findByDocid(ref[1:])
	}

	// Try virtual path (qmd://collection/path)
	if strings.HasPrefix(ref, "qmd://") {
		return s.findByVirtualPath(ref)
	}

	// Try as collection/filepath
	if parts := strings.SplitN(ref, "/", 2); len(parts) == 2 {
		doc, err := s.findByCollectionPath(parts[0], parts[1])
		if err == nil && doc != nil {
			return doc, nil
		}
	}

	// Try as filepath across all collections
	return s.findByFilepath(ref)
}

func (s *Store) findByDocid(prefix string) (*DocumentResult, error) {
	var d DocumentResult
	err := s.DB.QueryRow(`
		SELECT d.id, d.collection, d.filepath, d.title, d.hash, d.modified_at, d.body_length
		FROM documents d
		WHERE d.hash LIKE ? AND d.active=1
		LIMIT 1`,
		prefix+"%",
	).Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document with docid #%s not found", prefix)
	}
	if err != nil {
		return nil, err
	}
	d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
	return &d, nil
}

func (s *Store) findByVirtualPath(vpath string) (*DocumentResult, error) {
	// Parse qmd://collection/path
	rest := strings.TrimPrefix(vpath, "qmd://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid virtual path: %s", vpath)
	}
	return s.findByCollectionPath(parts[0], parts[1])
}

func (s *Store) findByCollectionPath(collection, filepath string) (*DocumentResult, error) {
	var d DocumentResult
	err := s.DB.QueryRow(`
		SELECT id, collection, filepath, title, hash, modified_at, body_length
		FROM documents
		WHERE collection=? AND filepath=? AND active=1`,
		collection, filepath,
	).Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength)
	if err == sql.ErrNoRows {
		// Try fuzzy match
		return s.fuzzyFind(collection, filepath)
	}
	if err != nil {
		return nil, err
	}
	d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
	return &d, nil
}

func (s *Store) findByFilepath(filepath string) (*DocumentResult, error) {
	var d DocumentResult
	err := s.DB.QueryRow(`
		SELECT id, collection, filepath, title, hash, modified_at, body_length
		FROM documents
		WHERE filepath=? AND active=1
		LIMIT 1`,
		filepath,
	).Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength)
	if err == sql.ErrNoRows {
		// Try suffix match
		return s.suffixFind(filepath)
	}
	if err != nil {
		return nil, err
	}
	d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
	return &d, nil
}

func (s *Store) suffixFind(suffix string) (*DocumentResult, error) {
	var d DocumentResult
	err := s.DB.QueryRow(`
		SELECT id, collection, filepath, title, hash, modified_at, body_length
		FROM documents
		WHERE filepath LIKE ? AND active=1
		LIMIT 1`,
		"%"+suffix,
	).Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document %q not found", suffix)
	}
	if err != nil {
		return nil, err
	}
	d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
	return &d, nil
}

func (s *Store) fuzzyFind(collection, filepath string) (*DocumentResult, error) {
	rows, err := s.DB.Query(`
		SELECT id, collection, filepath, title, hash, modified_at, body_length
		FROM documents
		WHERE collection=? AND active=1`,
		collection,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best *DocumentResult
	bestDist := 999

	for rows.Next() {
		var d DocumentResult
		if err := rows.Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength); err != nil {
			continue
		}
		dist := levenshtein(strings.ToLower(filepath), strings.ToLower(d.Filepath))
		if dist < bestDist && dist <= 3 {
			bestDist = dist
			d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
			cp := d
			best = &cp
		}
	}

	if best == nil {
		return nil, fmt.Errorf("document %s/%s not found", collection, filepath)
	}
	return best, nil
}

// GetDocumentBody returns the full body of a document, optionally sliced by line range.
func (s *Store) GetDocumentBody(doc *DocumentResult, fromLine, maxLines int) (string, error) {
	body, err := s.GetContentBody(doc.Hash)
	if err != nil {
		return "", err
	}

	if fromLine <= 0 && maxLines <= 0 {
		return body, nil
	}

	lines := strings.Split(body, "\n")
	start := 0
	if fromLine > 0 {
		start = fromLine - 1 // 1-indexed
		if start >= len(lines) {
			return "", nil
		}
	}
	end := len(lines)
	if maxLines > 0 && start+maxLines < end {
		end = start + maxLines
	}

	return strings.Join(lines[start:end], "\n"), nil
}

// FindDocuments finds documents matching a glob pattern or comma-separated list.
func (s *Store) FindDocuments(pattern string, collections []string) ([]DocumentResult, error) {
	// Check if it's a comma-separated list
	if strings.Contains(pattern, ",") && !strings.Contains(pattern, "*") {
		refs := strings.Split(pattern, ",")
		var results []DocumentResult
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			doc, err := s.FindDocument(ref)
			if err != nil {
				continue
			}
			results = append(results, *doc)
		}
		return results, nil
	}

	// Glob pattern match
	collFilter, collArgs := buildCollectionFilter(collections)
	q := `SELECT id, collection, filepath, title, hash, modified_at, body_length
	      FROM documents WHERE active=1` + collFilter

	rows, err := s.DB.Query(q, collArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DocumentResult
	for rows.Next() {
		var d DocumentResult
		if err := rows.Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength); err != nil {
			continue
		}
		d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
		// Match against collection/filepath or just filepath
		fullPath := d.Collection + "/" + d.Filepath
		matched, _ := doublestar.Match(pattern, fullPath)
		if !matched {
			matched, _ = doublestar.Match(pattern, d.Filepath)
		}
		if !matched {
			// Try matching against virtual path
			matched, _ = doublestar.Match(pattern, d.DisplayPath)
		}
		if matched {
			results = append(results, d)
		}
	}
	return results, nil
}

// ListFiles returns all active documents, optionally filtered by collection.
func (s *Store) ListFiles(collection string) ([]DocumentResult, error) {
	var rows *sql.Rows
	var err error
	if collection != "" {
		rows, err = s.DB.Query(`
			SELECT id, collection, filepath, title, hash, modified_at, body_length
			FROM documents WHERE active=1 AND collection=?
			ORDER BY filepath`, collection)
	} else {
		rows, err = s.DB.Query(`
			SELECT id, collection, filepath, title, hash, modified_at, body_length
			FROM documents WHERE active=1
			ORDER BY collection, filepath`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DocumentResult
	for rows.Next() {
		var d DocumentResult
		if err := rows.Scan(&d.ID, &d.Collection, &d.Filepath, &d.Title, &d.Hash, &d.ModifiedAt, &d.BodyLength); err != nil {
			continue
		}
		d.DisplayPath = BuildDisplayPath(d.Collection, d.Filepath)
		results = append(results, d)
	}
	return results, nil
}

// Status returns a summary of the store contents.
type StoreStatus struct {
	DBPath          string
	DBSize          int64
	TotalDocuments  int
	TotalContent    int
	TotalEmbeddings int
	Collections     map[string]int
}

func (s *Store) Status() (*StoreStatus, error) {
	st := &StoreStatus{
		DBPath:      s.DBPath,
		Collections: make(map[string]int),
	}

	// DB file size
	if info, err := s.statDBFile(); err == nil {
		st.DBSize = info
	}

	s.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE active=1").Scan(&st.TotalDocuments)
	s.DB.QueryRow("SELECT COUNT(*) FROM content").Scan(&st.TotalContent)
	s.DB.QueryRow("SELECT COUNT(DISTINCT hash) FROM embeddings").Scan(&st.TotalEmbeddings)

	rows, err := s.DB.Query("SELECT collection, COUNT(*) FROM documents WHERE active=1 GROUP BY collection")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count int
			rows.Scan(&name, &count)
			st.Collections[name] = count
		}
	}

	return st, nil
}

func (s *Store) statDBFile() (int64, error) {
	fi, err := os.Stat(s.DBPath)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// Cleanup removes orphaned content and inactive documents.
func (s *Store) Cleanup() (contentRemoved, docsRemoved int, err error) {
	// Delete inactive documents
	result, err := s.DB.Exec("DELETE FROM documents WHERE active=0")
	if err != nil {
		return 0, 0, err
	}
	docsRemoved64, _ := result.RowsAffected()
	docsRemoved = int(docsRemoved64)

	// Remove orphaned content (content not referenced by any document)
	result, err = s.DB.Exec(`
		DELETE FROM content WHERE hash NOT IN (
			SELECT DISTINCT hash FROM documents
		)`)
	if err != nil {
		return 0, docsRemoved, err
	}
	contentRemoved64, _ := result.RowsAffected()
	contentRemoved = int(contentRemoved64)

	// Remove orphaned embeddings
	s.DB.Exec(`
		DELETE FROM embeddings WHERE hash NOT IN (
			SELECT DISTINCT hash FROM documents WHERE active=1
		)`)

	// Clear LLM cache
	s.DB.Exec("DELETE FROM llm_cache")

	// Vacuum
	s.DB.Exec("VACUUM")

	return contentRemoved, docsRemoved, nil
}

// GetAllActiveHashes returns all distinct content hashes for active documents.
func (s *Store) GetAllActiveHashes() ([]string, error) {
	rows, err := s.DB.Query("SELECT DISTINCT hash FROM documents WHERE active=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var h string
		rows.Scan(&h)
		hashes = append(hashes, h)
	}
	return hashes, nil
}

// helpers

func AddLineNumbers(text string, startLine int) string {
	if startLine <= 0 {
		startLine = 1
	}
	lines := strings.Split(text, "\n")
	// Calculate width for line numbers
	maxLine := startLine + len(lines) - 1
	width := len(strconv.Itoa(maxLine))

	var sb strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&sb, "%*d: %s", width, startLine+i, line)
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

