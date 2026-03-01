package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/piranha/qqmd/config"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Re-index all collections",
	Run: func(cmd *cobra.Command, args []string) {
		pull, _ := cmd.Flags().GetBool("pull")

		colls, err := config.ListCollections()
		if err != nil {
			fatal("listing collections: %v", err)
		}
		if len(colls) == 0 {
			fmt.Println("No collections configured. Use 'qqmd collection add' first.")
			return
		}

		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		for _, coll := range colls {
			fmt.Printf("Updating collection %q...\n", coll.Name)

			// Run update command if configured
			if coll.Update != "" {
				fmt.Printf("  Running update command: %s\n", coll.Update)
				c := exec.Command("sh", "-c", coll.Update)
				c.Dir = expandPath(coll.Path)
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				if err := c.Run(); err != nil {
					fmt.Printf("  Warning: update command failed: %v\n", err)
				}
			}

			// Run git pull if --pull flag is set
			if pull {
				dir := expandPath(coll.Path)
				if isGitRepo(dir) {
					fmt.Printf("  Running git pull in %s\n", dir)
					c := exec.Command("git", "pull")
					c.Dir = dir
					c.Stdout = os.Stdout
					c.Stderr = os.Stderr
					if err := c.Run(); err != nil {
						fmt.Printf("  Warning: git pull failed: %v\n", err)
					}
				}
			}

			stats, err := s.IndexCollection(coll.Name, coll.Collection)
			if err != nil {
				fmt.Printf("  Error indexing %q: %v\n", coll.Name, err)
				continue
			}
			fmt.Printf("  %d added, %d updated, %d unchanged, %d removed\n",
				stats.Added, stats.Updated, stats.Unchanged, stats.Removed)
		}

		fmt.Println("Update complete.")
	},
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

func init() {
	updateCmd.Flags().Bool("pull", false, "Run git pull in collection directories before re-indexing")
	rootCmd.AddCommand(updateCmd)
}
