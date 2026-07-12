package cli

import (
	"context"
	"fmt"
	"sort"
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

	sweepIndexDropper = func(id string) error {
		return analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).DropIndex(id)
	}
)

type sweepAutoSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type sweepAutoResult struct {
	Candidates []stash.CleanupRecommendation `json:"candidates"`
	Dropped    []stash.CleanupRecommendation `json:"dropped"`
	Skipped    []sweepAutoSkip               `json:"skipped"`
	Failed     []stash.SweepFailure          `json:"failed"`
	Reclaimed  int64                         `json:"reclaimed"`
}

type sweepOutput struct {
	Expired        *stash.SweepResult            `json:"expired"`
	Auto           *stash.CleanupResult          `json:"auto"`
	AutoCandidates []stash.CleanupRecommendation `json:"auto_candidates"`
	AutoDropped    []stash.CleanupRecommendation `json:"auto_dropped"`
	AutoSkipped    []sweepAutoSkip               `json:"auto_skipped"`
	AutoFailed     []stash.SweepFailure          `json:"auto_failed"`
	AutoReclaimed  int64                         `json:"auto_reclaimed"`
}

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

--auto extends the report beyond expired TTL by running smart cleanup analysis
for orphaned, superseded, duplicate, and branch-gone categories. The "stale"
category is included only with --include-stale. With --apply, these categories
are auto-deleted only for documented regenerable cache tools (codemap/vecgrep);
evidence and generic checkpoints remain review-only. Like the standard sweep,
--auto is a dry-run unless --apply is also passed.`,
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

		// 1. Standard expired-TTL sweep.
		res, err := mgr.SweepExpiredFiltered(GetContext(), sweepApply, keepTag, sweepIncludeTag, sweepIndexDropper)
		if err != nil {
			return err
		}

		// 2. --auto: smart cleanup categories.
		var autoRes *stash.CleanupResult
		autoApply := sweepAutoResult{
			Candidates: []stash.CleanupRecommendation{},
			Dropped:    []stash.CleanupRecommendation{},
			Skipped:    []sweepAutoSkip{},
			Failed:     []stash.SweepFailure{},
		}
		if sweepAuto {
			autoRes, err = mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
				StaleDays: 30,
			})
			if err != nil {
				return err
			}
			autoApply = runAutoSweep(
				GetContext(), mgr, autoRes, sweepApply, keepTag, sweepIncludeTag,
				sweepIncludeStale, sweepIndexDropper,
			)
		}
		autoCandidates := autoApply.Candidates
		autoDropped := autoApply.Dropped
		failureCount := len(res.Failed) + len(autoApply.Failed)

		if printer.IsJSON() {
			if err := printer.JSON(sweepOutput{
				Expired:        res,
				Auto:           autoRes,
				AutoCandidates: autoCandidates,
				AutoDropped:    autoDropped,
				AutoSkipped:    autoApply.Skipped,
				AutoFailed:     autoApply.Failed,
				AutoReclaimed:  autoApply.Reclaimed,
			}); err != nil {
				return err
			}
			if failureCount > 0 {
				return fmt.Errorf("sweep failed for %d operation(s)", failureCount)
			}
			return nil
		}

		mode := "DRY RUN"
		if sweepApply {
			mode = "APPLIED"
		}
		if len(res.Expired) == 0 && (!sweepAuto || len(autoCandidates) == 0) && failureCount == 0 {
			printer.Success("Sweep %s: no expired stashes found.", mode)
			if sweepAuto && autoRes != nil && len(autoCandidates) == 0 {
				printer.Info("Auto cleanup: nothing to sweep.")
			}
			return nil
		}

		printer.Header(fmt.Sprintf("Sweep %s", mode))

		// Expired stashes. Applied output shows actual drops; dry-run shows the
		// plan. The result object retains both lists for automation.
		expiredShown := res.Expired
		if sweepApply {
			expiredShown = res.Dropped
		}
		if len(expiredShown) > 0 {
			printer.Section(fmt.Sprintf("Expired (%d)", len(expiredShown)))
			tbl := output.NewTable([]string{"ID"}, printer.IsQuiet())
			for _, id := range expiredShown {
				tbl.Append([]string{id})
			}
			tbl.Render()
		}

		// Auto-cleanup stashes. Dry-run shows the exact safe candidates; apply
		// shows what was actually removed.
		autoShown := autoCandidates
		section := "Auto candidates"
		if sweepApply {
			autoShown = autoDropped
			section = "Auto cleanup"
		}
		if sweepAuto && len(autoShown) > 0 {
			printer.Section(fmt.Sprintf("%s (%d)", section, len(autoShown)))
			tbl := output.NewTable([]string{"ID", "TOOL", "CATEGORY", "REASON", "SIZE"}, printer.IsQuiet())
			for _, rec := range autoShown {
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
		if len(expiredShown) > 0 {
			parts = append(parts, fmt.Sprintf("%d expired", len(expiredShown)))
		}
		if sweepAuto && len(autoShown) > 0 {
			byCat := make(map[stash.CleanupCategory]int)
			for _, rec := range autoShown {
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
		totalSwept := len(expiredShown) + len(autoShown)
		if len(parts) > 0 {
			printer.Println()
			verb := "Would sweep"
			if sweepApply {
				verb = "Swept"
			}
			printer.Info("%s %d stashes: %s", verb, totalSwept, strings.Join(parts, ", "))
		}

		if sweepApply && res.Reclaimed > 0 {
			printer.KeyValue("Reclaimed (expired)", formatSize(res.Reclaimed))
		}
		if sweepAuto && autoApply.Reclaimed > 0 {
			printer.KeyValue("Reclaimed (auto)", formatSize(autoApply.Reclaimed))
		}
		if !sweepApply {
			printer.Println()
			printer.Warn("This was a dry-run. Use --apply to actually drop these stashes.")
		}
		for _, failure := range res.Failed {
			printer.Warn("failed to %s stash %s: %s", failure.Stage, failure.ID, failure.Error)
		}
		for _, skipped := range autoApply.Skipped {
			printer.Warn("skipped auto-cleanup stash %s: %s", skipped.ID, skipped.Reason)
		}
		for _, failure := range autoApply.Failed {
			printer.Warn("failed to %s auto-cleanup stash %s: %s", failure.Stage, failure.ID, failure.Error)
		}
		if failureCount > 0 {
			return fmt.Errorf("sweep failed for %d operation(s)", failureCount)
		}
		return nil
	},
}

// runAutoSweep builds the complete, filtered smart-cleanup plan before applying
// it. This keeps the plan stable even when cancellation or a partial failure
// stops mutation partway through.
func runAutoSweep(
	ctx context.Context,
	mgr *stash.Manager,
	analysis *stash.CleanupResult,
	apply bool,
	keepTag string,
	includeTag string,
	includeStale bool,
	dropIndex func(id string) error,
) sweepAutoResult {
	out := sweepAutoResult{
		Candidates: []stash.CleanupRecommendation{},
		Dropped:    []stash.CleanupRecommendation{},
		Skipped:    []sweepAutoSkip{},
		Failed:     []stash.SweepFailure{},
	}
	if analysis == nil {
		return out
	}

	autoCategories := map[stash.CleanupCategory]bool{
		stash.CatOrphaned:   true,
		stash.CatSuperseded: true,
		stash.CatDuplicate:  true,
		stash.CatBranchGone: true,
	}
	if includeStale {
		autoCategories[stash.CatStale] = true
	}

	// Plan first. Filtering here, before any mutation, ensures the candidate list
	// is exactly what --include-tag and keep-tag protections allowed.
	for _, rec := range analysis.Recommendations {
		if rec.Category == stash.CatKeep || !autoCategories[rec.Category] || !smartCleanupAutoDeletable(rec) {
			continue
		}
		st, err := mgr.Info(ctx, rec.ID)
		if err != nil {
			out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "inspect", Error: err.Error()})
			continue
		}
		if keepTag != "" && st.Manifest.HasTag(keepTag) {
			out.Skipped = append(out.Skipped, sweepAutoSkip{ID: rec.ID, Reason: fmt.Sprintf("protected by keep tag %q", keepTag)})
			continue
		}
		if includeTag != "" && !st.Manifest.HasTag(includeTag) {
			continue
		}
		out.Candidates = append(out.Candidates, rec)
	}

	sort.Slice(out.Candidates, func(i, j int) bool { return out.Candidates[i].ID < out.Candidates[j].ID })
	if apply {
		for _, rec := range out.Candidates {
			if err := ctx.Err(); err != nil {
				out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "cancel", Error: err.Error()})
				continue
			}

			// Re-inspect immediately before deletion so a concurrent keep-tag or
			// include-tag change cannot bypass the plan's safety constraints.
			st, err := mgr.Info(ctx, rec.ID)
			if err != nil {
				out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "inspect", Error: err.Error()})
				continue
			}
			if keepTag != "" && st.Manifest.HasTag(keepTag) {
				out.Skipped = append(out.Skipped, sweepAutoSkip{ID: rec.ID, Reason: fmt.Sprintf("protected by keep tag %q", keepTag)})
				continue
			}
			if includeTag != "" && !st.Manifest.HasTag(includeTag) {
				out.Skipped = append(out.Skipped, sweepAutoSkip{ID: rec.ID, Reason: fmt.Sprintf("no longer has include tag %q", includeTag)})
				continue
			}
			if err := mgr.Drop(ctx, rec.ID); err != nil {
				out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "drop", Error: err.Error()})
				continue
			}

			out.Dropped = append(out.Dropped, rec)
			out.Reclaimed += rec.Size
			if dropIndex != nil {
				if err := dropIndex(rec.ID); err != nil {
					out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "index", Error: err.Error()})
				}
			}
		}
	}

	sort.Slice(out.Dropped, func(i, j int) bool { return out.Dropped[i].ID < out.Dropped[j].ID })
	sort.Slice(out.Skipped, func(i, j int) bool { return out.Skipped[i].ID < out.Skipped[j].ID })
	sort.Slice(out.Failed, func(i, j int) bool {
		if out.Failed[i].ID == out.Failed[j].ID {
			return out.Failed[i].Stage < out.Failed[j].Stage
		}
		return out.Failed[i].ID < out.Failed[j].ID
	})
	return out
}

func init() {
	sweepCmd.Flags().BoolVar(&sweepApply, "apply", false, "Actually drop expired stashes (default: dry-run)")
	sweepCmd.Flags().StringVar(&sweepKeepTag, "keep-tag", "", "Tag that exempts a stash from sweeping (default: keep)")
	sweepCmd.Flags().StringVar(&sweepIncludeTag, "include-tag", "", "Only sweep stashes with this tag (e.g. codemap-snapshot)")
	sweepCmd.Flags().BoolVar(&sweepAuto, "auto", false, "Also analyze smart-cleanup categories; requires --apply to delete safe cache candidates")
	sweepCmd.Flags().BoolVar(&sweepIncludeStale, "include-stale", false, "When --auto is set, also sweep stashes categorized as stale (default: exclude stale)")
}
