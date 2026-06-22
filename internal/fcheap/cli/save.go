package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	saveName  string
	saveTags  []string
	saveTool  string
	saveSource string
)

var saveCmd = &cobra.Command{
	Use:   "save <path>",
	Short: "Save a file or directory to the stash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		srcPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("source not found: %w", err)
		}

		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		opts := &stash.SaveOptions{
			SourcePath: srcPath,
			Name:       saveName,
			Tags:       saveTags,
			Tool:       saveTool,
		}

		st, err := mgr.Save(GetContext(), opts)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(st.Manifest)
		}

		printer.Success("Saved stash: %s", st.Manifest.ID)
		printer.KeyValue("Source", st.Manifest.SourcePath)
		printer.KeyValue("Files", fmt.Sprintf("%d", st.Manifest.FileCount))
		printer.KeyValue("Size", formatSize(st.Manifest.TotalSize))
		if st.Manifest.Tool != "" {
			printer.KeyValue("Tool", st.Manifest.Tool)
		}
		if len(st.Manifest.Tags) > 0 {
			printer.KeyValue("Tags", fmt.Sprintf("%v", st.Manifest.Tags))
		}
		if st.Manifest.BundleType != "generic" {
			printer.KeyValue("Bundle", st.Manifest.BundleType)
		}
		return nil
	},
}

func init() {
	saveCmd.Flags().StringVar(&saveName, "name", "", "Display name for the stash")
	saveCmd.Flags().StringSliceVar(&saveTags, "tag", nil, "Tags for categorization (comma-separated)")
	saveCmd.Flags().StringVar(&saveTool, "tool", "", "Tool that produced the content (e.g., vidtrace)")
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}