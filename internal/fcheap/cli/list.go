package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var listTag string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stashes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		stashes, err := mgr.List(GetContext(), listTag)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			type listItem struct {
				ID        string   `json:"id"`
				Name      string   `json:"name,omitempty"`
				Tool      string   `json:"tool,omitempty"`
				Tags      []string `json:"tags,omitempty"`
				FileCount int      `json:"file_count"`
				TotalSize int64    `json:"total_size"`
				CreatedAt string   `json:"created_at"`
			}
			var items []listItem
			for _, st := range stashes {
				items = append(items, listItem{
					ID:        st.Manifest.ID,
					Name:      st.Manifest.Name,
					Tool:      st.Manifest.Tool,
					Tags:      st.Manifest.Tags,
					FileCount: st.Manifest.FileCount,
					TotalSize: st.Manifest.TotalSize,
					CreatedAt: st.Manifest.CreatedAt,
				})
			}
			return printer.JSON(items)
		}

		if len(stashes) == 0 {
			printer.Println("No stashes found.")
			return nil
		}

		printer.Header(fmt.Sprintf("Stashes (%d)", len(stashes)))
		for _, st := range stashes {
			name := st.Manifest.Name
			if name == "" {
				name = st.Manifest.ID
			}
			printer.Printf("  %s\n", name)
			printer.KeyValue("ID", st.Manifest.ID)
			if st.Manifest.Tool != "" {
				printer.KeyValue("Tool", st.Manifest.Tool)
			}
			if len(st.Manifest.Tags) > 0 {
				printer.KeyValue("Tags", fmt.Sprintf("%v", st.Manifest.Tags))
			}
			printer.KeyValue("Files", fmt.Sprintf("%d (%s)", st.Manifest.FileCount, formatSize(st.Manifest.TotalSize)))
			printer.KeyValue("Created", st.Manifest.CreatedAt)
			if st.Manifest.BundleType != "generic" {
				printer.KeyValue("Bundle", st.Manifest.BundleType)
			}
			printer.Println("")
		}
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
}