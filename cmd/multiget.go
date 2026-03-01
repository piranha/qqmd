package cmd

import (
	"fmt"

	"github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/piranha/qqmd/store"
	"github.com/spf13/cobra"
)

var multiGetCmd = &cobra.Command{
	Use:   "multi-get <pattern>",
	Short: "Retrieve multiple documents by glob pattern or comma-separated list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		maxBytes, _ := cmd.Flags().GetInt("max-bytes")
		maxLines, _ := cmd.Flags().GetInt("lines")

		docs, err := s.FindDocuments(args[0], collections)
		if err != nil {
			fatal("finding documents: %v", err)
		}

		// Load bodies
		for i := range docs {
			body, err := s.GetDocumentBody(&docs[i], 0, maxLines)
			if err != nil {
				continue
			}
			if maxBytes > 0 && len(body) > maxBytes {
				docs[i].Body = ""
				continue
			}
			docs[i].Body = body
			docs[i].Context = config.FindContextForPath(docs[i].Collection, docs[i].Filepath)
		}

		// Filter out empty bodies if max-bytes was set
		if maxBytes > 0 {
			var filtered []store.DocumentResult
			for _, d := range docs {
				if d.Body != "" {
					filtered = append(filtered, d)
				}
			}
			docs = filtered
		}

		out := format.FormatDocuments(docs, getFormat(), format.Options{
			LineNumbers: lineNumbers,
		})
		fmt.Print(out)
	},
}

func init() {
	multiGetCmd.Flags().Int("max-bytes", 10240, "Skip files larger than this (0 = no limit)")
	multiGetCmd.Flags().IntP("lines", "l", 0, "Maximum lines per document")
	rootCmd.AddCommand(multiGetCmd)
}
