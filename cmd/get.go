package cmd

import (
	"fmt"
	"strings"

	"github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <file>",
	Short: "Retrieve a document by path, virtual path, or docid",
	Long:  "Retrieve a document by filepath, virtual path (qqmd://collection/path), or docid (#hash).\nOptionally specify line range with --from and -l flags.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		ref := args[0]
		fromLine, _ := cmd.Flags().GetInt("from")
		maxLines, _ := cmd.Flags().GetInt("lines")

		// Handle file:line syntax
		if idx := strings.LastIndex(ref, ":"); idx > 0 {
			lineStr := ref[idx+1:]
			var line int
			if _, err := fmt.Sscanf(lineStr, "%d", &line); err == nil {
				ref = ref[:idx]
				if fromLine == 0 {
					fromLine = line
				}
			}
		}

		doc, err := s.FindDocument(ref)
		if err != nil {
			fatal("%v", err)
		}

		body, err := s.GetDocumentBody(doc, fromLine, maxLines)
		if err != nil {
			fatal("reading body: %v", err)
		}
		doc.Body = body
		doc.Context = config.FindContextForPath(doc.Collection, doc.Filepath)

		out := format.FormatDocument(doc, getFormat(), format.Options{
			LineNumbers: lineNumbers,
		})
		fmt.Print(out)
	},
}

func init() {
	getCmd.Flags().Int("from", 0, "Start from line number (1-indexed)")
	getCmd.Flags().IntP("lines", "l", 0, "Maximum lines to return")
	rootCmd.AddCommand(getCmd)
}
