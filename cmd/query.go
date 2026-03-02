package cmd

import (
	"context"
	"fmt"
	"strings"

	configpkg "github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/piranha/qqmd/llm"
	"github.com/piranha/qqmd/store"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <query>",
	Short: "Hybrid search with expansion and reranking",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		opts := searchOpts()
		if len(opts.Collections) == 0 {
			defaults, _ := configpkg.GetDefaultCollectionNames()
			opts.Collections = defaults
		}

		query := args[0]
		ctx := context.Background()

		// Parse structured query (lex:, vec:, hyde: prefixes)
		lexQuery, vecQuery, hydeQuery := parseStructuredQuery(query)

		// Step 1: BM25 search with original or lex query
		ftsQuery := query
		if lexQuery != "" {
			ftsQuery = lexQuery
		}
		ftsOpts := opts
		ftsOpts.Limit = 40 // Get more candidates for fusion
		ftsResults, err := s.SearchFTS(ftsQuery, ftsOpts)
		if err != nil {
			fatal("FTS search: %v", err)
		}

		// Strong signal detection: if top BM25 result is very strong, skip expansion
		if len(ftsResults) > 0 && ftsResults[0].Score >= 0.85 {
			gap := 0.0
			if len(ftsResults) > 1 {
				gap = ftsResults[0].Score - ftsResults[1].Score
			} else {
				gap = ftsResults[0].Score
			}
			if gap >= 0.15 {
				// Strong signal — just return FTS results
				if len(ftsResults) > opts.EffectiveLimit() {
					ftsResults = ftsResults[:opts.EffectiveLimit()]
				}
				s.LoadDocumentBodies(ftsResults)
				for i := range ftsResults {
					ftsResults[i].Context = configpkg.FindContextForPath(ftsResults[i].Collection, ftsResults[i].Filepath)
				}
				out := format.FormatSearchResults(ftsResults, getFormat(), format.Options{
					Full:        showFull,
					Query:       query,
					LineNumbers: lineNumbers,
				})
				fmt.Print(out)
				return
			}
		}

		// Step 2: Try vector search if embeddings exist
		var vecResults []store.SearchResult

		embedder, err := llm.DefaultEmbedder()
		if err != nil {
			fatal("initializing embedder: %v", err)
		}
		defer llm.CloseIfNeeded(embedder)

		chatProvider, err := llm.DefaultChatProvider()
		if err != nil {
			// Chat provider is optional — reranking/expansion will be skipped
			chatProvider = nil
		}
		if chatProvider != nil {
			defer llm.CloseIfNeeded(chatProvider)
		}

		embCount, _, _ := s.EmbeddingStats()
		if embCount > 0 {
			// Determine vec query
			vecText := query
			if vecQuery != "" {
				vecText = vecQuery
			} else if hydeQuery != "" {
				vecText = hydeQuery
			}
			queryEmb := llm.FormatQueryForEmbedding(vecText)
			embedding, err := embedder.Embed(ctx, queryEmb)
			if err == nil {
				vecOpts := opts
				vecOpts.Limit = 40
				vecResults, _ = s.SearchVec(embedding, vecOpts)
			}
		}

		// Step 3: If no structured query and we have LLM, try query expansion
		if lexQuery == "" && vecQuery == "" && hydeQuery == "" && embCount > 0 && chatProvider != nil {
			expanded, err := chatProvider.ExpandQuery(ctx, query)
			if err == nil && expanded != nil {
				// Run expanded lex query
				if expanded.Lex != "" && expanded.Lex != query {
					expandedFTS, _ := s.SearchFTS(expanded.Lex, ftsOpts)
					ftsResults = append(ftsResults, expandedFTS...)
				}
				// Run expanded vec query
				if expanded.Vec != "" {
					vecText := llm.FormatQueryForEmbedding(expanded.Vec)
					emb, err := embedder.Embed(ctx, vecText)
					if err == nil {
						expandedVec, _ := s.SearchVec(emb, ftsOpts)
						vecResults = append(vecResults, expandedVec...)
					}
				}
			}
		}

		// Step 4: Hybrid fusion
		limit := opts.EffectiveLimit()
		results := s.HybridSearch(ftsResults, vecResults, limit*2)

		// Step 5: Rerank top candidates if we have enough and chat provider is available
		if len(results) > limit && chatProvider != nil {
			s.LoadDocumentBodies(results)
			var candidates []struct {
				Doc   string
				Index int
			}
			for i, r := range results {
				if i >= 40 {
					break
				}
				text := r.Body
				if len(text) > 1000 {
					text = text[:1000]
				}
				candidates = append(candidates, struct {
					Doc   string
					Index int
				}{Doc: text, Index: i})
			}
			reranked, err := llm.RerankResults(ctx, chatProvider, query, candidates)
			if err == nil && len(reranked) > 0 {
				var rerankedResults []store.SearchResult
				for _, idx := range reranked {
					if idx < len(results) {
						rerankedResults = append(rerankedResults, results[idx])
					}
				}
				results = rerankedResults
			}
		}

		if len(results) > limit {
			results = results[:limit]
		}

		s.LoadDocumentBodies(results)
		for i := range results {
			results[i].Context = configpkg.FindContextForPath(results[i].Collection, results[i].Filepath)
		}

		out := format.FormatSearchResults(results, getFormat(), format.Options{
			Full:        showFull,
			Query:       query,
			LineNumbers: lineNumbers,
		})
		fmt.Print(out)
	},
}

func parseStructuredQuery(query string) (lex, vec, hyde string) {
	lines := strings.Split(query, "\n")
	hasStructured := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "lex:") {
			lex = strings.TrimSpace(strings.TrimPrefix(line, "lex:"))
			hasStructured = true
		} else if strings.HasPrefix(line, "vec:") {
			vec = strings.TrimSpace(strings.TrimPrefix(line, "vec:"))
			hasStructured = true
		} else if strings.HasPrefix(line, "hyde:") {
			hyde = strings.TrimSpace(strings.TrimPrefix(line, "hyde:"))
			hasStructured = true
		}
	}
	if !hasStructured {
		return "", "", ""
	}
	return
}

func init() {
	rootCmd.AddCommand(queryCmd)
}
