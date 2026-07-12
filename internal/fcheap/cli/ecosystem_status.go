package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

type ecosystemToolStats struct {
	Count            int   `json:"count"`
	TotalSize        int64 `json:"total_size"` // deprecated alias for logical_size
	LogicalSize      int64 `json:"logical_size"`
	StoredSize       int64 `json:"stored_size"`
	OldestAgeSeconds int64 `json:"oldest_age_seconds"`
	Expired          int   `json:"expired"`
	Orphaned         int   `json:"orphaned"`
}

type ecosystemOverall struct {
	TotalStashes       int                  `json:"total_stashes"`
	TotalSize          int64                `json:"total_size"` // deprecated alias for logical_size
	LogicalSize        int64                `json:"logical_size"`
	StoredSize         int64                `json:"stored_size"`
	LogicalUsage       string               `json:"logical_usage"`
	DiskUsage          string               `json:"disk_usage"`
	ReclaimableSize    int64                `json:"reclaimable_size"`
	ReclaimableSavings string               `json:"reclaimable_savings"`
	CleanupResult      *stash.CleanupResult `json:"cleanup_result"`
}

type ecosystemStatusOutput struct {
	Tools   map[string]*ecosystemToolStats `json:"tools"`
	Overall ecosystemOverall               `json:"overall"`
}

var ecosystemStatusCmd = &cobra.Command{
	Use:   "ecosystem-status",
	Short: "Dashboard of all stashes grouped by tool, with cleanup savings estimate",
	Long: `Ecosystem-status lists all stashes grouped by their tool field, showing
per-tool counts, sizes, oldest age, and expired counts. The overall
summary includes total stashes, total size, and recommended cleanup
savings (from AnalyzeCleanup).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		// List all stashes including expired.
		stashes, err := mgr.ListFiltered(GetContext(), stash.ListOptions{IncludeExpired: true})
		if err != nil {
			return err
		}

		// Run AnalyzeCleanup for the recommended savings estimate.
		cleanupRes, err := mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
			StaleDays: 30,
		})
		if err != nil {
			return err
		}

		status := buildEcosystemStatus(stashes, cleanupRes, time.Now())
		byTool := status.Tools

		// Sort tools alphabetically (with "-" last).
		tools := make([]string, 0, len(byTool))
		for t := range byTool {
			tools = append(tools, t)
		}
		sort.Slice(tools, func(i, j int) bool {
			if tools[i] == "-" {
				return false
			}
			if tools[j] == "-" {
				return true
			}
			return tools[i] < tools[j]
		})

		if printer.IsJSON() {
			return printer.JSON(status)
		}

		printer.Header("Ecosystem Status")

		table := output.NewTable([]string{"TOOL", "COUNT", "STORED", "OLDEST", "EXPIRED", "ORPHANED"}, printer.IsQuiet())
		for _, tool := range tools {
			ts := byTool[tool]
			oldest := "-"
			if ts.OldestAgeSeconds > 0 {
				days := int((time.Duration(ts.OldestAgeSeconds) * time.Second).Hours() / 24)
				oldest = fmt.Sprintf("%dd", days)
			}
			table.Append([]string{
				tool,
				fmt.Sprintf("%d", ts.Count),
				formatSize(ts.StoredSize),
				oldest,
				fmt.Sprintf("%d", ts.Expired),
				fmt.Sprintf("%d", ts.Orphaned),
			})
		}
		table.Render()

		// Summary line.
		printer.Println()
		printer.Info("Total: %d stashes, %s logical", len(stashes), formatSize(status.Overall.LogicalSize))
		printer.KeyValue("Stored content estimate", formatSize(status.Overall.StoredSize))
		printer.KeyValue("Recommended cleanup savings", formatSize(cleanupRes.Reclaimable))

		return nil
	},
}

func init() {
	// No additional flags; --json is inherited from the root command.
}

func buildEcosystemStatus(stashes []*stash.Stash, cleanupRes *stash.CleanupResult, now time.Time) ecosystemStatusOutput {
	byTool := make(map[string]*ecosystemToolStats)
	var logicalSize, storedSize int64
	for _, st := range stashes {
		if st == nil || st.Manifest == nil {
			continue
		}
		tool := st.Manifest.Tool
		if tool == "" {
			tool = "-"
		}
		ts, ok := byTool[tool]
		if !ok {
			ts = &ecosystemToolStats{}
			byTool[tool] = ts
		}
		ts.Count++
		ts.TotalSize += st.Manifest.TotalSize
		ts.LogicalSize += st.Manifest.TotalSize
		logicalSize += st.Manifest.TotalSize
		stored := estimatedStoredSize(st.Manifest)
		ts.StoredSize += stored
		storedSize += stored

		if created, err := time.Parse(time.RFC3339, st.Manifest.CreatedAt); err == nil {
			ageSeconds := int64(now.Sub(created) / time.Second)
			if ageSeconds < 0 {
				ageSeconds = 0
			}
			if ageSeconds > ts.OldestAgeSeconds {
				ts.OldestAgeSeconds = ageSeconds
			}
		}
		if stash.IsExpired(st.Manifest) {
			ts.Expired++
		}
	}

	if cleanupRes == nil {
		cleanupRes = &stash.CleanupResult{
			Recommendations: []stash.CleanupRecommendation{},
			ByCategory:      map[stash.CleanupCategory]int{},
		}
	}
	for _, rec := range cleanupRes.Recommendations {
		if rec.Category != stash.CatOrphaned {
			continue
		}
		tool := rec.Tool
		if tool == "" {
			tool = "-"
		}
		if ts, ok := byTool[tool]; ok {
			ts.Orphaned++
		}
	}

	return ecosystemStatusOutput{
		Tools: byTool,
		Overall: ecosystemOverall{
			TotalStashes:       len(stashes),
			TotalSize:          logicalSize,
			LogicalSize:        logicalSize,
			StoredSize:         storedSize,
			LogicalUsage:       formatSize(logicalSize),
			DiskUsage:          formatSize(storedSize),
			ReclaimableSize:    cleanupRes.Reclaimable,
			ReclaimableSavings: formatSize(cleanupRes.Reclaimable),
			CleanupResult:      cleanupRes,
		},
	}
}

func estimatedStoredSize(man *manifest.Manifest) int64 {
	if man == nil {
		return 0
	}
	if man.Compression != "" && man.CompressedSize > 0 {
		return man.CompressedSize
	}
	return man.TotalSize
}
