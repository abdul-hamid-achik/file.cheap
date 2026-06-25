package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	sweepApply        bool
	sweepKeepTag      string
	sweepIncludeTag   string
	sweepAuto         bool
	sweepIncludeStale bool
)

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Find and optionally drop stashes whose TTL has expired (or, with --auto, smart-cleanup categories)",
	Long: `Sweep finds stashes whose time-to-live has elapsed and, when --apply is
passed, drops them (cleaning their DB rows and search index).

By default sweep is a dry-run: it reports which stashes would be dropped
without touching them. Use --apply to actually delete expired stashes.

Stashes with the --keep-tag tag (default: "keep") are never swept, even if
their TTL has expired — this is a safety net for pinning important stashes.

Use --include-tag to only sweep stashes bearing a specific tag (e.g.
--include-tag codemap-snapshot to only sweep regenerable snapshots).

--auto extends sweep beyond expired TTL: it runs the smart cleanup analysis
(AnalyzeCleanup) and also sweeps stashes categorized as orphaned,
superseded, duplicate, or branch-gone. The "stale" category is only swept
when --include-stale is also passed. The "keep" category is never swept.
--auto implies --apply (no dry-run when auto).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		keepTag := sweepKeepTag
		if keepTag == "" {
			keepTag = "keep" // default safety-net tag
		}

		// --auto implies --apply (no dry-run when auto).
		if sweepAuto {
			sweepApply = true
		}

		dropIndex := func(id string) error {
			return analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).DropIndex(id)
		}

		// 1. Standard expired-TTL sweep.
		res, err := mgr.SweepExpired(GetContext(), sweepApply, keepTag, dropIndex)
		if err != nil {
			return err
		}

		// Filter expired by include-tag if specified.
		if sweepIncludeTag != "" && len(res.Expired) > 0 {
			filtered := res.Expired[:0]
			for _, id := range res.Expired {
				st, err := mgr.Info(GetContext(), id)
				if err != nil {
					continue
				}
				if st.Manifest.HasTag(sweepIncludeTag) {
					filtered = append(filtered, id)
				}
			}
			res.Expired = filtered
		}

		// 2. --auto: smart cleanup categories.
		var autoRes *stash.CleanupResult
		var autoDropped []stash.CleanupRecommendation
		if sweepAuto {
			home, _ := os.UserHomeDir()
			projectsDir := filepath.Join(home, "projects")
			notesDir := filepath.Join(home, "notes", "projects")

			autoRes, err = mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
				StaleDays:   30,
				ProjectsDir: projectsDir,
				NotesDir:    notesDir,
			})
			if err != nil {
				return err
			}

			// Build the set of categories to sweep in auto mode.
			autoCats := map[stash.CleanupCategory]bool{
				stash.CatOrphaned:   true,
				stash.CatSuperseded: true,
				stash.CatDuplicate:  true,
				stash.CatBranchGone: true,
			}
			if sweepIncludeStale {
				autoCats[stash.CatStale] = true
			}

			an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)
			for _, rec := range autoRes.Recommendations {
				if rec.Category == stash.CatKeep {
					continue
				}
				if !autoCats[rec.Category] {
					continue
				}
				// Respect keep-tag: skip stashes bearing it.
				if keepTag != "" {
					if st, err := mgr.Info(GetContext(), rec.ID); err == nil && st.Manifest.HasTag(keepTag) {
						continue
					}
				}
				// Respect include-tag filter if set.
				if sweepIncludeTag != "" {
					st, err := mgr.Info(GetContext(), rec.ID)
					if err != nil || !st.Manifest.HasTag(sweepIncludeTag) {
						continue
					}
				}
				// Drop it.
				if err := mgr.Drop(GetContext(), rec.ID); err != nil {
					continue
				}
				_ = an.DropIndex(rec.ID)
				autoDropped = append(autoDropped, rec)
			}
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]any{
				"expired":      res,
				"auto":         autoRes,
				"auto_dropped": autoDropped,
			})
		}

		mode := "DRY RUN"
		if sweepApply {
			mode = "APPLIED"
		}
		if len(res.Expired) == 0 && (!sweepAuto || len(autoDropped) == 0) {
			printer.Success("Sweep %s: no expired stashes found.", mode)
			if sweepAuto && autoRes != nil && len(autoDropped) == 0 {
				printer.Info("Auto cleanup: nothing to sweep.")
			}
			return nil
		}

		printer.Header(fmt.Sprintf("Sweep %s", mode))

		// Expired stashes.
		if len(res.Expired) > 0 {
			printer.Section(fmt.Sprintf("Expired (%d)", len(res.Expired)))
			tbl := output.NewTable([]string{"ID"}, printer.IsQuiet())
			for _, id := range res.Expired {
				tbl.Append([]string{id})
			}
			tbl.Render()
		}

		// Auto-cleanup stashes.
		if sweepAuto && len(autoDropped) > 0 {
			printer.Section(fmt.Sprintf("Auto cleanup (%d)", len(autoDropped)))
			tbl := output.NewTable([]string{"ID", "TOOL", "CATEGORY", "REASON", "SIZE"}, printer.IsQuiet())
			for _, rec := range autoDropped {
				tool := rec.Tool
				if tool == "" {
					tool = "-"
				}
				tbl.Append([]string{
					rec.ID,
					tool,
					stash.CategoryDisplay(rec.Category),
					rec.Reason,
					formatSize(rec.Size),
				})
			}
			tbl.Render()
		}

		// Categorized summary line.
		var parts []string
		if len(res.Expired) > 0 {
			parts = append(parts, fmt.Sprintf("%d expired", len(res.Expired)))
		}
		if sweepAuto && len(autoDropped) > 0 {
			byCat := make(map[stash.CleanupCategory]int)
			for _, rec := range autoDropped {
				byCat[rec.Category]++
			}
			for _, cat := range []stash.CleanupCategory{
				stash.CatOrphaned,
				stash.CatSuperseded,
				stash.CatDuplicate,
				stash.CatBranchGone,
				stash.CatStale,
			} {
				if c, ok := byCat[cat]; ok && c > 0 {
					parts = append(parts, fmt.Sprintf("%d %s", c, stash.CategoryDisplay(cat)))
				}
			}
		}
		totalSwept := len(res.Expired) + len(autoDropped)
		if len(parts) > 0 {
			printer.Println()
			printer.Info("Swept %d stashes: %s", totalSwept, strings.Join(parts, ", "))
		}

		if sweepApply && res.Reclaimed > 0 {
			printer.KeyValue("Reclaimed (expired)", formatSize(res.Reclaimed))
		}
		if sweepAuto && len(autoDropped) > 0 {
			var autoReclaim int64
			for _, rec := range autoDropped {
				autoReclaim += rec.Size
			}
			printer.KeyValue("Reclaimed (auto)", formatSize(autoReclaim))
		}
		if !sweepApply {
			printer.Println()
			printer.Warn("This was a dry-run. Use --apply to actually drop these stashes.")
		}
		return nil
	},
}

func init() {
	sweepCmd.Flags().BoolVar(&sweepApply, "apply", false, "Actually drop expired stashes (default: dry-run)")
	sweepCmd.Flags().StringVar(&sweepKeepTag, "keep-tag", "", "Tag that exempts a stash from sweeping (default: keep)")
	sweepCmd.Flags().StringVar(&sweepIncludeTag, "include-tag", "", "Only sweep stashes with this tag (e.g. codemap-snapshot)")
	sweepCmd.Flags().BoolVar(&sweepAuto, "auto", false, "Also run smart cleanup analysis and sweep orphaned/superseded/duplicate/branch-gone stashes (implies --apply)")
	sweepCmd.Flags().BoolVar(&sweepIncludeStale, "include-stale", false, "When --auto is set, also sweep stashes categorized as stale (default: exclude stale)")
}
