package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/piranha/qqmd/config"
	"github.com/spf13/cobra"
)

var collectionCmd = &cobra.Command{
	Use:   "collection",
	Short: "Manage collections",
	Aliases: []string{"col"},
}

var collectionAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Index a directory as a collection",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		mask, _ := cmd.Flags().GetString("mask")

		if name == "" {
			// Default to directory basename
			path := args[0]
			for len(path) > 1 && path[len(path)-1] == '/' {
				path = path[:len(path)-1]
			}
			for i := len(path) - 1; i >= 0; i-- {
				if path[i] == '/' {
					name = path[i+1:]
					break
				}
			}
			if name == "" {
				name = path
			}
		}

		if !config.IsValidCollectionName(name) {
			fatal("invalid collection name %q (use alphanumeric, hyphens, underscores)", name)
		}

		if err := config.AddCollection(name, args[0], mask); err != nil {
			fatal("adding collection: %v", err)
		}
		fmt.Printf("Collection %q added at %s (pattern: %s)\n", name, args[0], mask)

		// Auto-index
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		coll, err := config.GetCollection(name)
		if err != nil || coll == nil {
			fatal("reading collection: %v", err)
		}
		stats, err := s.IndexCollection(name, coll.Collection)
		if err != nil {
			fatal("indexing: %v", err)
		}
		fmt.Printf("Indexed: %d added, %d updated, %d unchanged, %d removed\n",
			stats.Added, stats.Updated, stats.Unchanged, stats.Removed)
	},
}

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all collections",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		colls, err := config.ListCollections()
		if err != nil {
			fatal("listing collections: %v", err)
		}
		if len(colls) == 0 {
			fmt.Println("No collections configured.")
			return
		}

		format := getFormat()
		if format == "json" {
			type entry struct {
				Name             string `json:"name"`
				Path             string `json:"path"`
				Pattern          string `json:"pattern"`
				Update           string `json:"update,omitempty"`
				IncludeByDefault bool   `json:"include_by_default"`
			}
			entries := make([]entry, len(colls))
			for i, c := range colls {
				incl := true
				if c.IncludeByDefault != nil {
					incl = *c.IncludeByDefault
				}
				entries[i] = entry{
					Name:             c.Name,
					Path:             c.Path,
					Pattern:          c.Pattern,
					Update:           c.Update,
					IncludeByDefault: incl,
				}
			}
			data, _ := json.MarshalIndent(entries, "", "  ")
			fmt.Println(string(data))
		} else {
			for _, c := range colls {
				incl := "yes"
				if c.IncludeByDefault != nil && !*c.IncludeByDefault {
					incl = "no"
				}
				fmt.Printf("%-20s %s (pattern: %s, default: %s)\n", c.Name, c.Path, c.Pattern, incl)
			}
		}
	},
}

var collectionRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a collection",
	Aliases: []string{"rm"},
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		// Deactivate documents in store
		s, err := openStore()
		if err == nil {
			s.DeactivateCollection(name)
			s.Close()
		}
		if err := config.RemoveCollection(name); err != nil {
			fatal("removing collection: %v", err)
		}
		fmt.Printf("Collection %q removed\n", name)
	},
}

var collectionRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a collection",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if !config.IsValidCollectionName(args[1]) {
			fatal("invalid collection name %q", args[1])
		}
		if err := config.RenameCollection(args[0], args[1]); err != nil {
			fatal("renaming collection: %v", err)
		}
		fmt.Printf("Collection renamed: %s -> %s\n", args[0], args[1])
	},
}

var collectionShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show collection details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		coll, err := config.GetCollection(args[0])
		if err != nil {
			fatal("reading collection: %v", err)
		}
		if coll == nil {
			fatal("collection %q not found", args[0])
		}
		data, _ := json.MarshalIndent(coll, "", "  ")
		fmt.Println(string(data))
	},
}

var collectionSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Update collection settings",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var update *string
		var incl *bool
		if cmd.Flags().Changed("update-cmd") {
			v, _ := cmd.Flags().GetString("update-cmd")
			update = &v
		}
		if cmd.Flags().Changed("include") {
			v, _ := cmd.Flags().GetBool("include")
			incl = &v
		}
		if update == nil && incl == nil {
			fatal("specify --update-cmd or --include")
		}
		if err := config.UpdateCollectionSettings(args[0], update, incl); err != nil {
			fatal("updating collection: %v", err)
		}
		fmt.Printf("Collection %q updated\n", args[0])
	},
}

func init() {
	collectionAddCmd.Flags().String("name", "", "Collection name (default: directory basename)")
	collectionAddCmd.Flags().String("mask", "**/*.md", "File glob pattern")

	collectionSetCmd.Flags().String("update-cmd", "", "Shell command to run before re-indexing")
	collectionSetCmd.Flags().Bool("include", true, "Include in default queries")

	collectionCmd.AddCommand(collectionAddCmd, collectionListCmd, collectionRemoveCmd,
		collectionRenameCmd, collectionShowCmd, collectionSetCmd)
	rootCmd.AddCommand(collectionCmd)
}
