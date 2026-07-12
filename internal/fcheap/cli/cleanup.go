package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/cleanup"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

// smartCleanupFailure records an operation that could not be completed during
// an applied smart cleanup. Stage is "cancel", "inspect", "drop", or "index".
type smartCleanupFailure struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

// smartCleanupSkip records a cleanup recommendation that was protected by the
// configured keep tag or was not safe for unattended deletion. CatKeep
// recommendations are not included: they were never deletion candidates.
type smartCleanupSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// smartCleanupOutput is the stable machine-readable contract for smart cleanup.
// Recommendations and Reclaimable describe the pre-apply analysis; Dropped and
// Reclaimed describe what actually happened. This distinction keeps dry-run
// useful while ensuring --apply output never overstates successful deletion.
type smartCleanupOutput struct {
	Total           int                           `json:"total"`
	Recommendations []stash.CleanupRecommendation `json:"recommendations"`
	ByCategory      map[stash.CleanupCategory]int `json:"by_category"`
	Reclaimable     int64                         `json:"reclaimable"`
	Applied         bool                          `json:"applied"`
	Dropped         []string                      `json:"dropped"`
	Reclaimed       int64                         `json:"reclaimed"`
	Failed          []smartCleanupFailure         `json:"failed"`
	Skipped         []smartCleanupSkip            `json:"skipped"`
}

type smartCleanupApplyResult struct {
	Dropped   []string
	Reclaimed int64
	Failed    []smartCleanupFailure
	Skipped   []smartCleanupSkip
}

var (
	cleanupApply      bool
	cleanupKeepTag    string
	cleanupTool       string
	cleanupTag        string
	cleanupDropOnly   bool
	cleanupExpired    bool
	cleanupSmart      bool
	cleanupCategories []string
	cleanupStaleDays  int

	cleanupIndexDropper = func(id string) error {
		return analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).DropIndex(id)
	}
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Analyze stashes for cleanup using heuristics or smart categorization",
	Long: `Analyze stashes for cleanup. Two modes:

DEFAULT (scoring mode): scores each stash 0-100 on "droppability" using weighted
signals: source path gone (+35), cache tool like codemap/vecgrep (+25), evidence
tool like vidtrace/cairntrace (-30, protective), old age (+15), large size (+10),
keep tags (-50, protective), expired TTL (+40), content-hash dedup (+20).
Verdicts: drop (>=60), review (>=30), keep (<30).

  fcheap cleanup
  fcheap cleanup --drop-only
  fcheap cleanup --apply --tool codemap

SMART (category mode, --smart): categorizes every stash into exactly one
cleanup category based on why it might be droppable:

  expired     — TTL has elapsed
  orphaned    — recorded source path no longer exists
  superseded  — a newer stash exists for the same tool + source path
  duplicate   — same content hash as a newer stash
  branch-gone — a "branch:" tag references a deleted git branch
  stale       — older than --stale-days (uses created_at as proxy)
  keep        — no cleanup reason found

Priority order ensures each stash gets only its first matching category
(expired beats orphaned beats superseded, etc.) — no double counting.

  fcheap cleanup --smart
  fcheap cleanup --smart --categories expired,orphaned
  fcheap cleanup --smart --apply --categories expired,duplicate
  fcheap cleanup --smart --stale-days 30 --apply

By default cleanup is a dry-run: it reports candidates without dropping them.
Use --apply to drop safe candidates. Both modes auto-delete only explicit TTL
expirations or documented regenerable caches (codemap/vecgrep). Other DROP or
non-keep recommendations remain review-only and are reported as skipped.

Stashes with the --keep-tag tag (default: "keep") are never dropped in scoring
mode — this is a safety net for pinning important stashes.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if cleanupSmart {
			return runSmartCleanup(mgr)
		}
		return runScoringCleanup(mgr)
	},
}

// runScoringCleanup runs the scoring-based heuristic cleanup (default mode).
func runScoringCleanup(mgr *stash.Manager) error {
	keepTag := cleanupKeepTag
	if keepTag == "" {
		keepTag = "keep"
	}

	result, err := cleanup.Run(GetContext(), mgr, cleanupIndexDropper, cleanup.Options{
		Apply:    cleanupApply,
		KeepTag:  keepTag,
		Tool:     cleanupTool,
		Tag:      cleanupTag,
		DropOnly: cleanupDropOnly,
		Expired:  cleanupExpired,
	})
	if err != nil {
		return err
	}

	if printer.IsJSON() {
		if err := printer.JSON(result); err != nil {
			return err
		}
		if len(result.Failed) > 0 {
			return fmt.Errorf("cleanup failed for %d operation(s)", len(result.Failed))
		}
		return nil
	}

	mode := "DRY RUN"
	if cleanupApply {
		mode = "APPLIED"
	}

	if len(result.Candidates) == 0 {
		printer.Success("Cleanup %s: no candidates found.", mode)
		return nil
	}

	// Group by verdict for readability.
	drops, reviews, keeps := groupByVerdict(result.Candidates)

	printer.Header(fmt.Sprintf("Cleanup %s: %d candidate(s)", mode, len(result.Candidates)))

	if len(drops) > 0 {
		printer.Section(fmt.Sprintf("DROP (%d)", len(drops)))
		table := output.NewTable([]string{"ID", "SCORE", "SIZE", "REASONS"}, printer.IsQuiet())
		for _, c := range drops {
			table.Append([]string{c.StashID, fmt.Sprintf("%d", c.Score), formatSize(c.Size), cleanup.FormatReasons(c.Reasons)})
		}
		table.Render()
	}

	if len(reviews) > 0 {
		printer.Section(fmt.Sprintf("REVIEW (%d)", len(reviews)))
		table := output.NewTable([]string{"ID", "SCORE", "SIZE", "REASONS"}, printer.IsQuiet())
		for _, c := range reviews {
			table.Append([]string{c.StashID, fmt.Sprintf("%d", c.Score), formatSize(c.Size), cleanup.FormatReasons(c.Reasons)})
		}
		table.Render()
	}

	if len(keeps) > 0 {
		printer.Section(fmt.Sprintf("KEEP (%d)", len(keeps)))
		for _, c := range keeps {
			printer.Indent("%s (score %d)", c.StashID, c.Score)
		}
	}

	if cleanupApply && len(result.Dropped) > 0 {
		printer.Println()
		printer.Success("Dropped %d stash(es), reclaimed %s", len(result.Dropped), formatSize(result.Reclaimed))
	}
	if cleanupApply && len(result.Skipped) > 0 {
		printer.Warn("Skipped %d review-only stash(es); add an explicit TTL or target a documented cache tool", len(result.Skipped))
	}
	for _, failure := range result.Failed {
		printer.Warn("failed to %s stash %s: %s", failure.Stage, failure.ID, failure.Error)
	}

	if !cleanupApply && len(drops) > 0 {
		printer.Println()
		printer.Warn("This was a dry-run. Use --apply to drop stashes scored as DROP.")
	}

	if len(result.Failed) > 0 {
		return fmt.Errorf("cleanup failed for %d operation(s)", len(result.Failed))
	}
	return nil
}

// runSmartCleanup runs the category-based smart cleanup (--smart mode).
func runSmartCleanup(mgr *stash.Manager) error {
	result, err := mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
		StaleDays:  cleanupStaleDays,
		Categories: cleanupCategories,
	})
	if err != nil {
		return err
	}

	keepTag := cleanupKeepTag
	if keepTag == "" {
		keepTag = "keep"
	}

	applyResult := smartCleanupApplyResult{
		Dropped: []string{},
		Failed:  []smartCleanupFailure{},
		Skipped: []smartCleanupSkip{},
	}
	if cleanupApply {
		applyResult = applySmartCleanup(GetContext(), mgr, result, keepTag, cleanupIndexDropper)
	}

	out := smartCleanupOutput{
		Total:           result.Total,
		Recommendations: result.Recommendations,
		ByCategory:      result.ByCategory,
		Reclaimable:     result.Reclaimable,
		Applied:         cleanupApply,
		Dropped:         applyResult.Dropped,
		Reclaimed:       applyResult.Reclaimed,
		Failed:          applyResult.Failed,
		Skipped:         applyResult.Skipped,
	}
	if out.Recommendations == nil {
		out.Recommendations = []stash.CleanupRecommendation{}
	}
	if out.ByCategory == nil {
		out.ByCategory = map[stash.CleanupCategory]int{}
	}

	if printer.IsJSON() {
		if err := printer.JSON(out); err != nil {
			return err
		}
		if len(out.Failed) > 0 {
			return fmt.Errorf("smart cleanup failed for %d stash(es)", len(out.Failed))
		}
		return nil
	}

	if len(result.Recommendations) == 0 {
		printer.Success("Cleanup --smart: no stashes to analyze.")
		return nil
	}

	mode := "DRY RUN"
	if cleanupApply {
		mode = "APPLIED"
	}

	printer.Header(fmt.Sprintf("Cleanup --smart %s: %d stashes analyzed", mode, result.Total))

	// Build the output table: ID | TOOL | CATEGORY | REASON | SIZE
	table := output.NewTable([]string{"ID", "TOOL", "CATEGORY", "REASON", "SIZE"}, printer.IsQuiet())
	for _, rec := range result.Recommendations {
		tool := rec.Tool
		if tool == "" {
			tool = "-"
		}
		table.Append([]string{
			rec.ID,
			tool,
			stash.CategoryDisplay(rec.Category),
			rec.Reason,
			formatSize(rec.Size),
		})
	}
	table.Render()

	// Summary line.
	var parts []string
	for _, cat := range []stash.CleanupCategory{
		stash.CatExpired,
		stash.CatOrphaned,
		stash.CatSuperseded,
		stash.CatDuplicate,
		stash.CatBranchGone,
		stash.CatStale,
		stash.CatKeep,
	} {
		if count, ok := result.ByCategory[cat]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, stash.CategoryDisplay(cat)))
		}
	}
	summary := fmt.Sprintf("%d stashes analyzed: %s. Reclaimable: %s",
		result.Total,
		strings.Join(parts, ", "),
		formatSize(result.Reclaimable))
	printer.Println()
	printer.Info("%s", summary)

	if cleanupApply {
		for _, skipped := range out.Skipped {
			printer.Warn("skipped stash %s: %s", skipped.ID, skipped.Reason)
		}
		for _, failed := range out.Failed {
			printer.Warn("failed to %s stash %s: %s", failed.Stage, failed.ID, failed.Error)
		}
		if len(out.Dropped) > 0 {
			printer.Println()
			printer.Success("Dropped %d stash(es), reclaimed %s", len(out.Dropped), formatSize(out.Reclaimed))
		}
	} else {
		printer.Println()
		printer.Warn("This was a dry-run. Use --apply to drop non-keep stashes.")
	}

	if len(out.Failed) > 0 {
		return fmt.Errorf("smart cleanup failed for %d stash(es)", len(out.Failed))
	}
	return nil
}

// applySmartCleanup executes a previously computed smart-cleanup plan. It is
// deliberately independent of output mode: --json changes serialization only,
// never whether deletion happens.
func applySmartCleanup(
	ctx context.Context,
	mgr *stash.Manager,
	result *stash.CleanupResult,
	keepTag string,
	dropIndex func(id string) error,
) smartCleanupApplyResult {
	out := smartCleanupApplyResult{
		Dropped: []string{},
		Failed:  []smartCleanupFailure{},
		Skipped: []smartCleanupSkip{},
	}
	for _, rec := range result.Recommendations {
		if rec.Category == stash.CatKeep {
			continue
		}

		// Inspect before deleting so an unreadable manifest cannot bypass keep-tag
		// protection. Treat an inspection failure as a failed operation, not consent
		// to delete.
		st, err := mgr.Info(ctx, rec.ID)
		if err != nil {
			out.Failed = append(out.Failed, smartCleanupFailure{
				ID: rec.ID, Stage: "inspect", Error: err.Error(),
			})
			continue
		}
		if keepTag != "" && st.Manifest.HasTag(keepTag) {
			out.Skipped = append(out.Skipped, smartCleanupSkip{
				ID: rec.ID, Reason: fmt.Sprintf("protected by keep tag %q", keepTag),
			})
			continue
		}
		if !smartCleanupAutoDeletable(rec) {
			out.Skipped = append(out.Skipped, smartCleanupSkip{
				ID:     rec.ID,
				Reason: "requires review: only explicitly expired or regenerable cache stashes are auto-deletable",
			})
			continue
		}
		if err := ctx.Err(); err != nil {
			out.Failed = append(out.Failed, smartCleanupFailure{
				ID: rec.ID, Stage: "cancel", Error: err.Error(),
			})
			continue
		}

		if err := mgr.Drop(ctx, rec.ID); err != nil {
			out.Failed = append(out.Failed, smartCleanupFailure{
				ID: rec.ID, Stage: "drop", Error: err.Error(),
			})
			continue
		}
		out.Dropped = append(out.Dropped, rec.ID)
		out.Reclaimed += rec.Size
		if dropIndex != nil {
			if err := dropIndex(rec.ID); err != nil {
				out.Failed = append(out.Failed, smartCleanupFailure{
					ID: rec.ID, Stage: "index", Error: err.Error(),
				})
			}
		}
	}

	// Deterministic machine output even if filesystem iteration order changes.
	sort.Strings(out.Dropped)
	sort.Slice(out.Failed, func(i, j int) bool { return out.Failed[i].ID < out.Failed[j].ID })
	sort.Slice(out.Skipped, func(i, j int) bool { return out.Skipped[i].ID < out.Skipped[j].ID })
	return out
}

// smartCleanupAutoDeletable is intentionally conservative. A missing source,
// old branch, duplicate payload, or newer checkpoint may be a useful evidence
// trail rather than disposable data. Expired TTLs are explicit retention
// intent, while these tool types are documented regenerable caches.
func smartCleanupAutoDeletable(rec stash.CleanupRecommendation) bool {
	if rec.Category == stash.CatExpired {
		return true
	}
	switch rec.Tool {
	case "codemap", "vecgrep":
		return true
	default:
		return false
	}
}

func groupByVerdict(candidates []cleanup.Candidate) (drops, reviews, keeps []cleanup.Candidate) {
	for _, c := range candidates {
		switch c.Verdict {
		case "drop":
			drops = append(drops, c)
		case "review":
			reviews = append(reviews, c)
		default:
			keeps = append(keeps, c)
		}
	}
	return
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupApply, "apply", false, "Actually drop stashes (default: dry-run)")
	cleanupCmd.Flags().StringVar(&cleanupKeepTag, "keep-tag", "", "Tag that exempts a stash from cleanup (default: keep)")
	cleanupCmd.Flags().StringVar(&cleanupTool, "tool", "", "Only analyze stashes from this tool")
	cleanupCmd.Flags().StringVar(&cleanupTag, "tag", "", "Only analyze stashes with this tag")
	cleanupCmd.Flags().BoolVar(&cleanupDropOnly, "drop-only", false, "Only show stashes scored as drop (default: show all)")
	cleanupCmd.Flags().BoolVar(&cleanupExpired, "expired", false, "Include stashes with an expired TTL (sweep handles these by default)")
	cleanupCmd.Flags().BoolVar(&cleanupSmart, "smart", false, "Use category-based smart analysis (expired/orphaned/superseded/duplicate/branch-gone/stale/keep)")
	cleanupCmd.Flags().StringSliceVar(&cleanupCategories, "categories", nil, "Smart mode: filter to specific categories (comma-separated: expired,orphaned,superseded,duplicate,branch-gone,stale,keep)")
	cleanupCmd.Flags().IntVar(&cleanupStaleDays, "stale-days", 0, "Smart mode: days without access to be considered stale (0 = disabled)")
}
