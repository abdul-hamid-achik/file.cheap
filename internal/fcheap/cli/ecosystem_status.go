package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

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
		home, _ := os.UserHomeDir()
		projectsDir := filepath.Join(home, "projects")
		notesDir := filepath.Join(home, "notes", "projects")
		cleanupRes, err := mgr.AnalyzeCleanup(GetContext(), stash.CleanupOptions{
			StaleDays:   30,
			ProjectsDir: projectsDir,
			NotesDir:    notesDir,
		})
		if err != nil {
			return err
		}

		// Per-tool aggregation.
		type toolStats struct {
			Count     int
			Size      int64
			OldestAge time.Duration
			Expired   int
			Orphaned  int
		}
		byTool := make(map[string]*toolStats)

		var totalSize int64
		for _, st := range stashes {
			tool := st.Manifest.Tool
			if tool == "" {
				tool = "-"
			}
			ts, ok := byTool[tool]
			if !ok {
				ts = &toolStats{}
				byTool[tool] = ts
			}
			ts.Count++
			ts.Size += st.Manifest.TotalSize
			totalSize += st.Manifest.TotalSize

			// Oldest age.
			if st.Manifest.CreatedAt != "" {
				if created, perr := time.Parse(time.RFC3339, st.Manifest.CreatedAt); perr == nil {
					age := time.Since(created)
					if ts.OldestAge == 0 || age > ts.OldestAge {
						ts.OldestAge = age
					}
				}
			}

			// Expired count.
			if stash.IsExpired(st.Manifest) {
				ts.Expired++
			}
		}

		// Orphaned count per tool from cleanup result.
		for _, rec := range cleanupRes.Recommendations {
			if rec.Category == stash.CatOrphaned {
				tool := rec.Tool
				if tool == "" {
					tool = "-"
				}
				if ts, ok := byTool[tool]; ok {
					ts.Orphaned++
				}
			}
		}

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
			return printer.JSON(map[string]any{
				"tools": byTool,
				"overall": map[string]any{
					"total_stashes":       len(stashes),
					"total_size":          totalSize,
					"disk_usage":          formatSize(totalSize),
					"reclaimable_savings": formatSize(cleanupRes.Reclaimable),
					"cleanup_result":      cleanupRes,
				},
			})
		}

		printer.Header("Ecosystem Status")

		table := output.NewTable([]string{"TOOL", "COUNT", "SIZE", "OLDEST", "EXPIRED", "ORPHANED"}, printer.IsQuiet())
		for _, tool := range tools {
			ts := byTool[tool]
			oldest := "-"
			if ts.OldestAge > 0 {
				days := int(ts.OldestAge.Hours() / 24)
				oldest = fmt.Sprintf("%dd", days)
			}
			table.Append([]string{
				tool,
				fmt.Sprintf("%d", ts.Count),
				formatSize(ts.Size),
				oldest,
				fmt.Sprintf("%d", ts.Expired),
				fmt.Sprintf("%d", ts.Orphaned),
			})
		}
		table.Render()

		// Summary line.
		printer.Println()
		printer.Info("Total: %d stashes, %s total", len(stashes), formatSize(totalSize))
		printer.KeyValue("Disk usage estimate", formatSize(totalSize))
		printer.KeyValue("Recommended cleanup savings", formatSize(cleanupRes.Reclaimable))

		return nil
	},
}

func init() {
	// No additional flags; --json is inherited from the root command.
}
