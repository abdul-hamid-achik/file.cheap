package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var analyzeQuery string

var analyzeCmd = &cobra.Command{
	Use:   "analyze <stash-id>",
	Short: "Index a stash for search and run analysis",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if !mgr.Exists(args[0]) {
			return fmt.Errorf("stash not found: %s", args[0])
		}

		stashDir := mgr.StashDir(args[0])
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)

		if err := an.IndexStash(GetContext(), stashDir); err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]string{
				"stash_id": args[0],
				"status":   "indexed",
			})
		}

		printer.Success("Indexed stash: %s", args[0])

		// If query is provided, search within the stash
		if analyzeQuery != "" {
			results, err := an.SearchStash(GetContext(), stashDir, analyzeQuery)
			if err != nil {
				return err
			}
			if len(results) > 0 {
				printer.Section("Search Results")
				for _, r := range results {
					printer.KeyValue("Score", fmt.Sprintf("%.2f", r.Score))
					printer.Indent("%s", truncate(r.Text, 200))
				}
			} else {
				printer.Println("No matches found.")
			}
		}
		return nil
	},
}

func init() {
	analyzeCmd.Flags().StringVar(&analyzeQuery, "query", "", "Search query to run within the stash")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}