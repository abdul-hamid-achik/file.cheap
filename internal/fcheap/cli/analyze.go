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
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).WithEmbedder(embSettings())

		res, err := an.IndexStash(GetContext(), stashDir)
		if err != nil {
			return err
		}

		if printer.IsJSON() && analyzeQuery == "" {
			return printer.JSON(res)
		}

		if !printer.IsJSON() {
			printer.Success("Indexed stash: %s", res.StashID)
			printer.KeyValue("Files indexed", fmt.Sprintf("%d", res.FilesIndex))
			if res.BundleType != "generic" {
				printer.KeyValue("Bundle", res.BundleType)
			}
		}

		// If a query is provided, search within the stash.
		if analyzeQuery != "" {
			results, err := an.SearchStash(GetContext(), stashDir, analyzeQuery, 0, "")
			if err != nil {
				return err
			}
			if printer.IsJSON() {
				return printer.JSON(map[string]any{"index": res, "results": results})
			}
			if len(results) > 0 {
				printer.Section(fmt.Sprintf("Search Results (%d)", len(results)))
				for _, r := range results {
					label := r.File
					if label == "" {
						label = "(derived)"
					}
					printer.KeyValue(label, fmt.Sprintf("score %.2f", r.Score))
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
