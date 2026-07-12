package cli

import (
	"errors"
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	searchLimit int
	searchStash string
	searchMode  string
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across all stashes (or one with --stash)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).WithEmbedder(embSettings())

		// Search stash content (keyword / semantic / hybrid), optionally scoped to
		// one stash. To search a codebase instead, use `fcheap connect`.
		var results []analyze.SearchResult
		if searchStash != "" {
			if !mgr.Exists(searchStash) {
				return fmt.Errorf("stash not found: %s", searchStash)
			}
			results, err = an.SearchStash(GetContext(), mgr.StashDir(searchStash), args[0], searchLimit, searchMode)
		} else {
			results, err = an.Search(GetContext(), args[0], searchLimit, searchMode)
		}
		if err != nil {
			if errors.Is(err, analyze.ErrNotIndexed) {
				// Not indexed is data (empty), not a tool failure: exit 0.
				if printer.IsJSON() {
					return printer.JSON([]any{})
				}
				if searchStash != "" {
					printer.Println(fmt.Sprintf("Stash %s is not indexed. Run 'fcheap analyze %s' to make it searchable.", searchStash, searchStash))
				} else {
					printer.Println("No stashes are indexed yet. Run 'fcheap analyze <stash-id>' to make a stash searchable.")
				}
				return nil
			}
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(results)
		}

		if len(results) == 0 {
			printer.Println("No matches found. Make sure stashes are indexed with 'fcheap analyze <stash-id>'.")
			return nil
		}

		printer.Header(fmt.Sprintf("Search Results (%d)", len(results)))
		for _, r := range results {
			loc := r.StashID
			if r.File != "" {
				loc = fmt.Sprintf("%s  ›  %s", r.StashID, r.File)
			}
			printer.Section(loc)
			printer.KeyValue("Score", fmt.Sprintf("%.2f (%s)", r.Score, r.Source))
			printer.Indent("%s", truncate(r.Text, 300))
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Maximum number of results")
	searchCmd.Flags().StringVar(&searchStash, "stash", "", "Limit the search to a single stash ID")
	searchCmd.Flags().StringVar(&searchMode, "mode", "", "Search mode: keyword, semantic, hybrid (default: hybrid if an embedder is configured, else keyword)")
}
