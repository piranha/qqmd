package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean orphaned data, vacuum database",
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		contentRemoved, docsRemoved, err := s.Cleanup()
		if err != nil {
			fatal("cleanup failed: %v", err)
		}

		fmt.Printf("Cleanup complete: %d inactive documents removed, %d orphaned content entries removed\n",
			docsRemoved, contentRemoved)
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}
