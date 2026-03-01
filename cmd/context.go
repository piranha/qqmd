package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piranha/qqmd/config"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage context descriptions for collections",
}

var contextAddCmd = &cobra.Command{
	Use:   "add <path> <description>",
	Short: "Add context description for a path",
	Long:  "Add a context description using a virtual path (qmd://collection/path) or collection path pair.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		desc := args[1]

		collection, pathPrefix := parseContextPath(path)
		if collection == "" {
			fatal("path must use qmd://collection/path format")
		}

		if err := config.AddContext(collection, pathPrefix, desc); err != nil {
			fatal("adding context: %v", err)
		}
		fmt.Printf("Context added for %s/%s\n", collection, pathPrefix)
	},
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contexts",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		entries, err := config.ListAllContexts()
		if err != nil {
			fatal("listing contexts: %v", err)
		}

		format := getFormat()
		if format == "json" {
			data, _ := json.MarshalIndent(entries, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(entries) == 0 {
				fmt.Println("No contexts configured.")
				return
			}
			for _, e := range entries {
				fmt.Printf("%-15s %-20s %s\n", e.Collection, e.Path, e.Description)
			}
		}
	},
}

var contextRemoveCmd = &cobra.Command{
	Use:   "rm <collection> <path>",
	Short: "Remove a context",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.RemoveContext(args[0], args[1]); err != nil {
			fatal("removing context: %v", err)
		}
		fmt.Printf("Context removed for %s/%s\n", args[0], args[1])
	},
}

func parseContextPath(path string) (collection, pathPrefix string) {
	if strings.HasPrefix(path, "qmd://") {
		rest := strings.TrimPrefix(path, "qmd://")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[0], "/" + parts[1]
		}
		return parts[0], "/"
	}
	// Try collection/path format
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 {
		return parts[0], "/" + parts[1]
	}
	return path, "/"
}

func init() {
	contextCmd.AddCommand(contextAddCmd, contextListCmd, contextRemoveCmd)
	rootCmd.AddCommand(contextCmd)
}
