package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	listTag   string
	listTool  string
	listSince string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stashes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		opts := stash.ListOptions{Tag: listTag, Tool: listTool}
		if listSince != "" {
			since, err := stash.ParseSince(listSince)
			if err != nil {
				return err
			}
			opts.Since = since
		}

		stashes, err := mgr.ListFiltered(GetContext(), opts)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			type listItem struct {
				ID          string   `json:"id"`
				Name        string   `json:"name,omitempty"`
				Tool        string   `json:"tool,omitempty"`
				Tags        []string `json:"tags,omitempty"`
				FileCount   int      `json:"file_count"`
				TotalSize   int64    `json:"total_size"`
				Compression string   `json:"compression,omitempty"`
				CreatedAt   string   `json:"created_at"`
			}
			items := make([]listItem, 0, len(stashes))
			for _, st := range stashes {
				items = append(items, listItem{
					ID:          st.Manifest.ID,
					Name:        st.Manifest.Name,
					Tool:        st.Manifest.Tool,
					Tags:        st.Manifest.Tags,
					FileCount:   st.Manifest.FileCount,
					TotalSize:   st.Manifest.TotalSize,
					Compression: st.Manifest.Compression,
					CreatedAt:   st.Manifest.CreatedAt,
				})
			}
			return printer.JSON(items)
		}

		if len(stashes) == 0 {
			printer.Println("No stashes found. Use 'fcheap save <path>' to create one.")
			return nil
		}

		printer.Header(fmt.Sprintf("Stashes (%d)", len(stashes)))
		table := output.NewTable([]string{"ID", "TOOL", "TAGS", "FILES", "SIZE", "AGE", "COMP"}, printer.IsQuiet())
		for _, st := range stashes {
			m := st.Manifest
			tool := m.Tool
			if tool == "" {
				tool = "-"
			}
			tags := strings.Join(m.Tags, ",")
			if tags == "" {
				tags = "-"
			}
			table.Append([]string{
				m.ID,
				tool,
				tags,
				fmt.Sprintf("%d", m.FileCount),
				formatSize(m.TotalSize),
				humanAge(m.CreatedAt),
				compLabel(m.Compression),
			})
		}
		table.Render()
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
	listCmd.Flags().StringVar(&listTool, "tool", "", "Filter by tool")
	listCmd.Flags().StringVar(&listSince, "since", "", "Only show stashes newer than e.g. 24h, 7d, 2w, or 2026-06-01")
}

// compLabel renders a short compression indicator.
func compLabel(compression string) string {
	switch compression {
	case "zstd":
		return "zst"
	case "gzip":
		return "gz"
	case "none":
		return "tar"
	case "":
		return "-"
	default:
		return compression
	}
}

// humanAge formats an RFC3339 timestamp as a compact relative age.
func humanAge(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
