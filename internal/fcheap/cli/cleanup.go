package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/cleanup"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	cleanupApply       bool
	cleanupKeepTag     string
	cleanupTool        string
	cleanupTag         string
	cleanupDropOnly    bool
	cleanupExpired     bool
	cleanupSmart       bool
	cleanupCategories  []string
	cleanupStaleDays   int
	cleanupProjectsDir string
	cleanupNotesDir    string
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
  orphaned    — source path (or project directory) no longer exists
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
Use --apply to actually drop stashes. In scoring mode, only "drop" verdicts are
dropped. In smart mode, all non-keep stashes are dropped (respecting --categories).

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

	an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)
	result, err := cleanup.Run(GetContext(), mgr, an.DropIndex, cleanup.Options{
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
		return printer.JSON(result)
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

	if !cleanupApply && len(drops) > 0 {
		printer.Println()
		printer.Warn("This was a dry-run. Use --apply to drop stashes scored as DROP.")
	}

	return nil
}

// runSmartCleanup runs the category-based smart cleanup (--smart mode).
func runSmartCleanup(mgr *stash.Manager) error {
	// Resolve default directories.
	projectsDir := cleanupProjectsDir
	if projectsDir == "" {
		home, _ := os.UserHomeDir()
		projectsDir = filepath.Join(home, "projects")
	}
	notesDir := cleanupNotesDir
	if notesDir == "" {
		home, _ := os.UserHomeDir()
		notesDir = filepath.Join(home, "notes", "projects")
	}

	result, err := mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
		StaleDays:   cleanupStaleDays,
		ProjectsDir: projectsDir,
		NotesDir:    notesDir,
		Categories:  cleanupCategories,
	})
	if err != nil {
		return err
	}

	if printer.IsJSON() {
		return printer.JSON(result)
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

	// Apply: drop all non-keep recommendations (respecting --categories filter
	// and --keep-tag).
	keepTag := cleanupKeepTag
	if keepTag == "" {
		keepTag = "keep"
	}
	if cleanupApply {
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)
		var dropped []string
		for _, rec := range result.Recommendations {
			if rec.Category == stash.CatKeep {
				continue
			}
			// Respect keep-tag: skip stashes bearing it.
			if st, err := mgr.Info(GetContext(), rec.ID); err == nil && st.Manifest.HasTag(keepTag) {
				continue
			}
			if err := mgr.Drop(GetContext(), rec.ID); err != nil {
				printer.Warn("failed to drop stash %s: %v", rec.ID, err)
				continue
			}
			_ = an.DropIndex(rec.ID)
			dropped = append(dropped, rec.ID)
		}
		if len(dropped) > 0 {
			printer.Println()
			printer.Success("Dropped %d stash(es), reclaimed %s", len(dropped), formatSize(result.Reclaimable))
		}
	} else {
		printer.Println()
		printer.Warn("This was a dry-run. Use --apply to drop non-keep stashes.")
	}

	return nil
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
	cleanupCmd.Flags().StringVar(&cleanupProjectsDir, "projects-dir", "", "Smart mode: path to ~/projects for orphan detection (default: ~/projects)")
	cleanupCmd.Flags().StringVar(&cleanupNotesDir, "notes-dir", "", "Smart mode: path to ~/notes/projects for orphan detection (default: ~/notes/projects)")
}
