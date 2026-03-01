package store

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
)

// SearchOptions configures search behavior.
type SearchOptions struct {
	Collections []string
	Limit       int
	MinScore    float64
	ShowAll     bool
}

func (o *SearchOptions) EffectiveLimit() int {
	if o.ShowAll {
		return 10000
	}
	if o.Limit <= 0 {
		return 5
	}
	return o.Limit
}

// SearchFTS performs BM25 full-text search.
func (s *Store) SearchFTS(query string, opts SearchOptions) ([]SearchResult, error) {
	ftsQuery := buildFTS5Query(query)
	limit := opts.EffectiveLimit()

	// Build collection filter
	collFilter, collArgs := buildCollectionFilter(opts.Collections)

	q := `SELECT d.id, d.collection, d.filepath, d.title, d.hash, d.modified_at, d.body_length,
	             bm25(documents_fts, 10.0, 1.0, 1.0) as score
	      FROM documents_fts f
	      JOIN documents d ON d.id = f.rowid
	      WHERE documents_fts MATCH ?
	        AND d.active = 1` + collFilter + `
	      ORDER BY score
	      LIMIT ?`

	args := []any{ftsQuery}
	args = append(args, collArgs...)
	args = append(args, limit)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var rawScore float64
		if err := rows.Scan(&r.ID, &r.Collection, &r.Filepath, &r.Title,
			&r.Hash, &r.ModifiedAt, &r.BodyLength, &rawScore); err != nil {
			return nil, err
		}
		// bm25 returns negative values; negate for positive score
		r.Score = normalizeBM25(rawScore)
		r.Source = "fts"
		r.DisplayPath = BuildDisplayPath(r.Collection, r.Filepath)
		results = append(results, r)
	}

	// Normalize scores so best result = 1.0
	if len(results) > 0 {
		maxScore := results[0].Score
		for _, r := range results[1:] {
			if r.Score > maxScore {
				maxScore = r.Score
			}
		}
		if maxScore > 0 {
			for i := range results {
				results[i].Score = results[i].Score / maxScore
			}
		}
	}

	// Filter by min score
	if opts.MinScore > 0 {
		filtered := results[:0]
		for _, r := range results {
			if r.Score >= opts.MinScore {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	return results, nil
}

// SearchVec performs vector similarity search using stored embeddings.
func (s *Store) SearchVec(queryEmbedding []float32, opts SearchOptions) ([]SearchResult, error) {
	limit := opts.EffectiveLimit()

	// Load all embeddings and compute cosine similarity
	collFilter, collArgs := buildCollectionFilter(opts.Collections)

	q := `SELECT e.hash, e.seq, e.chunk_start, e.chunk_end, e.embedding,
	             d.id, d.collection, d.filepath, d.title, d.modified_at, d.body_length
	      FROM embeddings e
	      JOIN documents d ON d.hash = e.hash AND d.active = 1` + collFilter + `
	      WHERE e.embedding IS NOT NULL`

	rows, err := s.DB.Query(q, collArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		result   SearchResult
		chunkPos int
	}
	var candidates []scored

	for rows.Next() {
		var (
			hash, collection, filepath, title, modifiedAt string
			seq, chunkStart, chunkEnd, bodyLength         int
			embBlob                                       []byte
			docID                                         int64
		)
		if err := rows.Scan(&hash, &seq, &chunkStart, &chunkEnd, &embBlob,
			&docID, &collection, &filepath, &title, &modifiedAt, &bodyLength); err != nil {
			continue
		}
		emb := decodeEmbedding(embBlob)
		if emb == nil {
			continue
		}
		sim := cosineSimilarity(queryEmbedding, emb)
		// Convert cosine similarity to 0-1 score (cosine is already -1 to 1, shift to 0-1)
		score := (sim + 1.0) / 2.0

		candidates = append(candidates, scored{
			result: SearchResult{
				DocumentResult: DocumentResult{
					ID:          docID,
					Collection:  collection,
					Filepath:    filepath,
					DisplayPath: BuildDisplayPath(collection, filepath),
					Title:       title,
					Hash:        hash,
					ModifiedAt:  modifiedAt,
					BodyLength:  bodyLength,
				},
				Score:    score,
				Source:   "vec",
				ChunkPos: chunkStart,
			},
		})
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].result.Score > candidates[j].result.Score
	})

	// Deduplicate by document (keep best score per doc)
	seen := make(map[int64]bool)
	var results []SearchResult
	for _, c := range candidates {
		if seen[c.result.ID] {
			continue
		}
		seen[c.result.ID] = true
		results = append(results, c.result)
		if len(results) >= limit {
			break
		}
	}

	// Filter by min score
	if opts.MinScore > 0 {
		filtered := results[:0]
		for _, r := range results {
			if r.Score >= opts.MinScore {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	return results, nil
}

// HybridSearch combines FTS and vector search using Reciprocal Rank Fusion.
func (s *Store) HybridSearch(ftsResults, vecResults []SearchResult, limit int) []SearchResult {
	const k = 60

	type docScore struct {
		result SearchResult
		rrf    float64
	}

	scores := make(map[int64]*docScore)

	// FTS contribution
	for rank, r := range ftsResults {
		weight := 1.0
		ds, ok := scores[r.ID]
		if !ok {
			ds = &docScore{result: r}
			scores[r.ID] = ds
		}
		ds.rrf += weight / float64(k+rank+1)
		// Top-rank bonus
		if rank == 0 {
			ds.rrf += 0.05
		} else if rank <= 2 {
			ds.rrf += 0.02
		}
	}

	// Vec contribution
	for rank, r := range vecResults {
		weight := 1.0
		ds, ok := scores[r.ID]
		if !ok {
			ds = &docScore{result: r}
			scores[r.ID] = ds
		}
		ds.rrf += weight / float64(k+rank+1)
		if rank == 0 {
			ds.rrf += 0.05
		} else if rank <= 2 {
			ds.rrf += 0.02
		}
	}

	// Collect and sort
	var combined []docScore
	for _, ds := range scores {
		combined = append(combined, *ds)
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].rrf > combined[j].rrf
	})

	// Normalize scores to 0-1
	var maxRRF float64
	if len(combined) > 0 {
		maxRRF = combined[0].rrf
	}

	var results []SearchResult
	for i, c := range combined {
		if i >= limit {
			break
		}
		r := c.result
		if maxRRF > 0 {
			r.Score = c.rrf / maxRRF
		}
		r.Source = "hybrid"
		results = append(results, r)
	}

	return results
}

// LoadDocumentBodies populates the Body field on search results.
func (s *Store) LoadDocumentBodies(results []SearchResult) error {
	for i := range results {
		body, err := s.GetContentBody(results[i].Hash)
		if err != nil {
			continue
		}
		results[i].Body = body
	}
	return nil
}

// GetContentBody returns the body text for a content hash.
func (s *Store) GetContentBody(hash string) (string, error) {
	var body string
	err := s.DB.QueryRow("SELECT body FROM content WHERE hash=?", hash).Scan(&body)
	return body, err
}

// HasEmbeddings returns true if any embeddings exist for the given hash.
func (s *Store) HasEmbeddings(hash string) bool {
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM embeddings WHERE hash=?", hash).Scan(&count)
	return count > 0
}

// StoreEmbedding stores a chunk embedding.
func (s *Store) StoreEmbedding(hash string, seq, chunkStart, chunkEnd int, embedding []float32, model string) error {
	blob := encodeEmbedding(embedding)
	_, err := s.DB.Exec(
		`INSERT OR REPLACE INTO embeddings (hash, seq, chunk_start, chunk_end, embedding, model)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		hash, seq, chunkStart, chunkEnd, blob, model,
	)
	return err
}

// DeleteEmbeddings removes all embeddings for a content hash.
func (s *Store) DeleteEmbeddings(hash string) error {
	_, err := s.DB.Exec("DELETE FROM embeddings WHERE hash=?", hash)
	return err
}

// EmbeddingStats returns counts of embedded vs total documents.
func (s *Store) EmbeddingStats() (embedded, total int, err error) {
	err = s.DB.QueryRow(`
		SELECT COUNT(DISTINCT hash) FROM embeddings
	`).Scan(&embedded)
	if err != nil {
		return
	}
	err = s.DB.QueryRow(`
		SELECT COUNT(DISTINCT hash) FROM documents WHERE active=1
	`).Scan(&total)
	return
}

// helpers

func buildFTS5Query(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return query
	}

	// If user already wrote FTS5 syntax (quotes, OR, AND, NEAR), pass through
	if strings.ContainsAny(query, `"()`) || strings.Contains(query, " OR ") ||
		strings.Contains(query, " AND ") || strings.Contains(query, "NEAR") {
		return query
	}

	words := strings.Fields(query)
	if len(words) == 0 {
		return query
	}

	// Build three-tier query: "exact phrase" OR (NEAR terms) OR (term1 OR term2)
	// This gives best results for casual search queries
	exact := `"` + strings.Join(words, " ") + `"`
	if len(words) == 1 {
		return exact
	}

	near := "NEAR(" + strings.Join(words, " ") + ", 10)"
	orTerms := strings.Join(words, " OR ")

	return exact + " OR " + near + " OR " + orTerms
}

func buildCollectionFilter(collections []string) (string, []any) {
	if len(collections) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(collections))
	args := make([]any, len(collections))
	for i, c := range collections {
		placeholders[i] = "?"
		args[i] = c
	}
	return " AND d.collection IN (" + strings.Join(placeholders, ",") + ")", args
}

func normalizeBM25(raw float64) float64 {
	abs := math.Abs(raw)
	return abs / (1.0 + abs)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func encodeEmbedding(emb []float32) []byte {
	buf := make([]byte, 4*len(emb))
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeEmbedding(data []byte) []float32 {
	if len(data)%4 != 0 {
		return nil
	}
	emb := make([]float32, len(data)/4)
	for i := range emb {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return emb
}

// ExtractSnippet finds the best matching snippet in body for the given query.
func ExtractSnippet(body, query string, maxLen int) (snippet string, line int) {
	if body == "" {
		return "", 0
	}
	if maxLen <= 0 {
		maxLen = 300
	}

	lines := strings.Split(body, "\n")
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	// Score each line by how many query terms it contains
	bestScore := 0
	bestLine := 0
	for i, l := range lines {
		lower := strings.ToLower(l)
		score := 0
		for _, w := range queryWords {
			if len(w) >= 3 && strings.Contains(lower, w) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestLine = i
		}
	}

	// Extract context around best line
	start := bestLine - 2
	if start < 0 {
		start = 0
	}
	end := bestLine + 3
	if end > len(lines) {
		end = len(lines)
	}

	snippetLines := lines[start:end]
	result := strings.Join(snippetLines, "\n")
	if len(result) > maxLen {
		result = result[:maxLen] + "..."
	}
	return result, bestLine + 1
}
