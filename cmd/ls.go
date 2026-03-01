package cmd

import (
	"fmt"

	"github.com/piranha/qqmd/format"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls [collection]",
	Short: "List indexed files",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		collection := ""
		if len(args) > 0 {
			collection = args[0]
		}

		docs, err := s.ListFiles(collection)
		if err != nil {
			fatal("listing files: %v", err)
		}

		out := format.FormatFileList(docs, getFormat())
		fmt.Print(out)
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
