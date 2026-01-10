package cli

import (
	"fmt"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List uploaded files",
	Long: `List files in your file.cheap account.

Examples:
  fc list                    # List recent files
  fc list --limit=50         # List 50 files
  fc list --status=completed # Filter by status
  fc list --search=product   # Search by filename
  fc list --json | jq '.files[].url'`,
	RunE: runList,
}

var (
	listLimit  int
	listOffset int
	listStatus string
	listSearch string
)

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Number of files to list (max 100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Offset for pagination")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (pending, processing, completed, failed)")
	listCmd.Flags().StringVar(&listSearch, "search", "", "Search by filename")
}

func runList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	ctx := GetContext()
	resp, err := apiClient.ListFiles(ctx, listLimit, listOffset, listStatus, listSearch)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	if jsonOutput {
		return printer.JSON(resp)
	}

	if len(resp.Files) == 0 {
		printer.Info("No files found")
		return nil
	}

	table := output.NewTable([]string{"ID", "Filename", "Status", "Size", "Created"}, quietMode)

	for _, f := range resp.Files {
		table.Append([]string{
			f.ID[:8] + "...",
			truncate(f.Filename, 30),
			f.Status,
			formatSize(f.SizeBytes),
			formatTime(f.CreatedAt),
		})
	}

	table.Render()

	if !quietMode {
		printer.Println()
		printer.Printf("Showing %d of %d files", len(resp.Files), resp.Total)
		if resp.HasMore {
			printer.Printf(" (use --offset=%d for more)", listOffset+listLimit)
		}
		printer.Println()
	}

	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("Jan 2, 2006")
	}
}
