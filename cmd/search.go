package cmd

import (
	"fmt"

	"github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search (BM25)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		opts := searchOpts()
		if len(opts.Collections) == 0 {
			defaults, _ := config.GetDefaultCollectionNames()
			opts.Collections = defaults
		}

		results, err := s.SearchFTS(args[0], opts)
		if err != nil {
			fatal("search failed: %v", err)
		}

		// Load bodies for snippet extraction
		s.LoadDocumentBodies(results)

		// Load contexts
		for i := range results {
			results[i].Context = config.FindContextForPath(results[i].Collection, results[i].Filepath)
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
	rootCmd.AddCommand(searchCmd)
}
