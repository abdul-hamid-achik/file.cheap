package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <stash-id>",
	Short: "Show detailed info about a stash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		st, err := mgr.Info(GetContext(), args[0])
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(st.Manifest)
		}

		printer.Header("Stash Info")
		printer.KeyValue("ID", st.Manifest.ID)
		if st.Manifest.Name != "" {
			printer.KeyValue("Name", st.Manifest.Name)
		}
		printer.KeyValue("Created", st.Manifest.CreatedAt)
		if st.Manifest.SourcePath != "" {
			printer.KeyValue("Source", st.Manifest.SourcePath)
		}
		if st.Manifest.Tool != "" {
			printer.KeyValue("Tool", st.Manifest.Tool)
		}
		if st.Manifest.BundleType != "" {
			printer.KeyValue("Bundle", st.Manifest.BundleType)
		}
		printer.KeyValue("Files", fmt.Sprintf("%d", st.Manifest.FileCount))
		printer.KeyValue("Size", formatSize(st.Manifest.TotalSize))
		printer.KeyValue("Content Hash", st.Manifest.ContentHash)
		if st.Manifest.Compression != "" {
			printer.KeyValue("Compression", st.Manifest.Compression)
			printer.KeyValue("Compressed Size", formatSize(st.Manifest.CompressedSize))
		}
		if len(st.Manifest.Tags) > 0 {
			printer.KeyValue("Tags", fmt.Sprintf("%v", st.Manifest.Tags))
		}
		if len(st.Manifest.Custom) > 0 {
			printer.Section("Custom Metadata")
			for k, v := range st.Manifest.Custom {
				printer.KeyValue(k, v)
			}
		}
		if len(st.Manifest.Files) > 0 {
			printer.Section(fmt.Sprintf("Files (%d)", len(st.Manifest.Files)))
			maxShow := 20
			for i, f := range st.Manifest.Files {
				if i >= maxShow {
					printer.Indent("... and %d more", len(st.Manifest.Files)-maxShow)
					break
				}
				printer.Indent("%s (%s)", f.Path, formatSize(f.Size))
			}
		}
		return nil
	},
}