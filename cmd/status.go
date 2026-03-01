package cmd

import (
	"fmt"

	"github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/format"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index status and health",
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		st, err := s.Status()
		if err != nil {
			fatal("getting status: %v", err)
		}

		f := getFormat()
		if f == "json" {
			fmt.Println(format.FormatStatus(st, f))
		} else {
			fmt.Printf("Database: %s\n", st.DBPath)
			fmt.Printf("Size: %s\n", humanSize(st.DBSize))
			fmt.Printf("Documents: %d\n", st.TotalDocuments)
			fmt.Printf("Unique content: %d\n", st.TotalContent)
			fmt.Printf("Embedded: %d\n", st.TotalEmbeddings)
			fmt.Println()
			fmt.Println("Collections:")
			for name, count := range st.Collections {
				fmt.Printf("  %-20s %d files\n", name, count)
			}

			// Config info
			fmt.Printf("\nConfig: %s\n", config.ConfigPath())
		}
	},
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
