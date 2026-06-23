package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	connectQuery string
	connectLimit int
	connectIndex bool
	connectMode  string
)

var connectCmd = &cobra.Command{
	Use:   "connect <stash-id> <codebase-dir>",
	Short: "Connect a stash to a codebase: find the code that likely owns the bug",
	Long: `connect runs semantic code search (vecgrep) over a codebase using the
stashed artifact's text — e.g. a vidtrace bug report's OCR and transcript —
surfacing the file:line candidates most likely related to the bug.

This is the connective tissue: stash a repro, then point it at the live repo.

Examples:
  fcheap connect OPG-15061 ~/projects/graphite
  fcheap connect OPG-15061 ~/projects/graphite --index --limit 5
  fcheap connect OPG-15061 ~/projects/graphite --query "login token refresh"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, codebase := args[0], args[1]

		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}
		if !mgr.Exists(id) {
			return fmt.Errorf("stash not found: %s", id)
		}
		stashDir := mgr.StashDir(id)
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)

		query := connectQuery
		if query == "" {
			q, err := an.StashQuery(stashDir, 2000)
			if err != nil {
				return fmt.Errorf("derive query from stash: %w", err)
			}
			if q == "" {
				return fmt.Errorf("stash has no searchable text; pass --query")
			}
			query = q
		}

		matches, err := an.VecgrepSearchIn(GetContext(), codebase, query, connectLimit, connectIndex, connectMode)
		if err != nil {
			return err
		}

		res := &analyze.ConnectResult{StashID: id, Codebase: codebase, Query: query, Matches: matches}
		if printer.IsJSON() {
			return printer.JSON(res)
		}

		printer.Header(fmt.Sprintf("Connect %s → %s", id, codebase))
		printer.KeyValue("Query", truncate(query, 80))
		if len(matches) == 0 {
			printer.Println("No related code found. Try --index to (re)build the codebase index, or refine --query.")
			return nil
		}
		printer.Section(fmt.Sprintf("Candidate code (%d)", len(matches)))
		for _, m := range matches {
			printer.KeyValue(m.File, fmt.Sprintf("score %.2f", m.Score))
			printer.Indent("%s", truncate(m.Text, 160))
		}
		return nil
	},
}

func init() {
	connectCmd.Flags().StringVar(&connectQuery, "query", "", "Override the auto-extracted query")
	connectCmd.Flags().IntVar(&connectLimit, "limit", 10, "Max code matches")
	connectCmd.Flags().BoolVar(&connectIndex, "index", false, "Build the vecgrep index for the codebase first")
	connectCmd.Flags().StringVar(&connectMode, "mode", "", "vecgrep search mode: semantic, keyword, hybrid (default: vecgrep's default, hybrid)")
}
