package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across all stashes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		_ = mgr
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)

		// First try keyword search
		results, err := an.Search(GetContext(), args[0])
		if err != nil {
			return err
		}

		// Also try vecgrep if available
		vgrepResults, err := an.SearchWithVecgrep(GetContext(), args[0])
		if err != nil {
			// vecgrep not available, that's fine
			vgrepResults = nil
		}

		allResults := append(results, vgrepResults...)

		if printer.IsJSON() {
			return printer.JSON(allResults)
		}

		if len(allResults) == 0 {
			printer.Println("No matches found. Make sure stashes are indexed with 'fcheap analyze <stash-id>'.")
			return nil
		}

		printer.Header(fmt.Sprintf("Search Results (%d)", len(allResults)))
		for _, r := range allResults {
			printer.Section(r.StashID)
			printer.KeyValue("Source", r.Source)
			printer.KeyValue("Score", fmt.Sprintf("%.2f", r.Score))
			if r.File != "" {
				printer.KeyValue("File", r.File)
			}
			printer.Indent("%s", truncate(r.Text, 300))
		}
		return nil
	},
}