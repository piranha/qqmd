package cmd

import (
	"context"
	"fmt"

	configpkg "github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/piranha/qqmd/llm"
	"github.com/spf13/cobra"
)

var vsearchCmd = &cobra.Command{
	Use:   "vsearch <query>",
	Short: "Vector semantic search",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		provider, err := llm.DefaultProvider(true)
		if err != nil {
			fatal("initializing LLM: %v", err)
		}
		defer llm.CloseProvider(provider)

		ctx := context.Background()

		// Embed the query
		queryText := llm.FormatQueryForEmbedding(args[0])
		embedding, err := provider.Embed(ctx, queryText)
		if err != nil {
			fatal("embedding query: %v", err)
		}

		opts := searchOpts()
		if len(opts.Collections) == 0 {
			defaults, _ := configpkg.GetDefaultCollectionNames()
			opts.Collections = defaults
		}

		results, err := s.SearchVec(embedding, opts)
		if err != nil {
			fatal("vector search failed: %v", err)
		}

		s.LoadDocumentBodies(results)
		for i := range results {
			results[i].Context = configpkg.FindContextForPath(results[i].Collection, results[i].Filepath)
		}

		out := format.FormatSearchResults(results, getFormat(), format.Options{
			Full:        showFull,
			Query:       args[0],
			LineNumbers: lineNumbers,
		})
		fmt.Print(out)
	},
}

func init() {
	rootCmd.AddCommand(vsearchCmd)
}
