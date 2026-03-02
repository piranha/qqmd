// Package cmd implements the CLI commands for qqmd.
package cmd

import (
	"fmt"
	"os"

	"github.com/piranha/qqmd/config"
	"github.com/piranha/qqmd/store"
	"github.com/spf13/cobra"
)

var (
	outputJSON  bool
	outputCSV   bool
	outputMD    bool
	outputXML   bool
	outputFiles bool
	resultLimit int
	collections []string
	showAll     bool
	minScore    float64
	showFull    bool
	lineNumbers bool
	indexName   string
)

// Version is set via -ldflags at build time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "qqmd",
	Short: "On-device search engine for markdown files",
	Long:  "qqmd indexes markdown files and provides full-text, vector, and hybrid search.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if indexName != "" {
			config.SetIndexName(indexName)
		}
	},
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.BoolVar(&outputJSON, "json", false, "Output as JSON")
	pf.BoolVar(&outputCSV, "csv", false, "Output as CSV")
	pf.BoolVar(&outputMD, "md", false, "Output as Markdown")
	pf.BoolVar(&outputXML, "xml", false, "Output as XML")
	pf.BoolVar(&outputFiles, "files", false, "Output as simple file list")
	pf.IntVarP(&resultLimit, "limit", "n", 5, "Number of results")
	pf.StringArrayVarP(&collections, "collection", "c", nil, "Filter by collection (repeatable)")
	pf.BoolVar(&showAll, "all", false, "Return all matches")
	pf.Float64Var(&minScore, "min-score", 0, "Minimum relevance score (0-1)")
	pf.BoolVar(&showFull, "full", false, "Show full document content")
	pf.BoolVar(&lineNumbers, "line-numbers", false, "Add line numbers to output")
	pf.StringVar(&indexName, "index", "", "Named index to use")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getFormat() string {
	switch {
	case outputCSV:
		return "csv"
	case outputMD:
		return "md"
	case outputXML:
		return "xml"
	case outputFiles:
		return "files"
	default:
		return "json"
	}
}

func openStore() (*store.Store, error) {
	return store.Open(store.DefaultDBPath())
}

func fatal(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func searchOpts() store.SearchOptions {
	return store.SearchOptions{
		Collections: collections,
		Limit:       resultLimit,
		MinScore:    minScore,
		ShowAll:     showAll,
	}
}
