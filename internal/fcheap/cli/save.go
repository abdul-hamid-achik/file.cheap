package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	saveName       string
	saveTags       []string
	saveTool       string
	saveSource     string
	saveNoScan     bool
	saveNoCompress bool
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
			NoScan:     saveNoScan,
		}
		// Optional provenance: the original artifact this stash derives from
		// (e.g. the source video for a vidtrace bundle).
		if saveSource != "" {
			opts.Custom = map[string]string{"source": saveSource}
		}

		st, err := mgr.Save(GetContext(), opts)
		if err != nil {
			return err
		}

		// Auto-compress large stashes to reclaim disk space (configurable via
		// compress_threshold; opt out with --no-compress).
		autoCompressed := false
		if !saveNoCompress && cfg.CompressThreshold > 0 && st.Manifest.TotalSize >= cfg.CompressThreshold {
			if cres, cerr := mgr.Compress(GetContext(), st.Manifest.ID, cfg.Compression); cerr == nil {
				st.Manifest.Compression = cres.Algorithm
				st.Manifest.CompressedSize = cres.CompressedSize
				autoCompressed = true
			}
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
		if autoCompressed {
			printer.KeyValue("Compressed", fmt.Sprintf("%s → %s (%s)", formatSize(st.Manifest.TotalSize), formatSize(st.Manifest.CompressedSize), st.Manifest.Compression))
		}
		if len(st.Secrets) > 0 {
			printer.Warn("%d potential secret(s) detected in this stash — review before sharing or restoring elsewhere", len(st.Secrets))
			shown := 0
			for _, f := range st.Secrets {
				if shown >= 5 {
					printer.Indent("... and %d more", len(st.Secrets)-shown)
					break
				}
				printer.Indent("%s:%d [%s]", f.File, f.Line, f.Rule)
				shown++
			}
		}
		return nil
	},
}

func init() {
	saveCmd.Flags().StringVar(&saveName, "name", "", "Display name for the stash")
	saveCmd.Flags().StringSliceVar(&saveTags, "tag", nil, "Tags for categorization (comma-separated)")
	saveCmd.Flags().StringVar(&saveTool, "tool", "", "Tool that produced the content (e.g., vidtrace)")
	saveCmd.Flags().StringVar(&saveSource, "source", "", "Original artifact this stash derives from (provenance)")
	saveCmd.Flags().BoolVar(&saveNoScan, "no-scan", false, "Skip the save-time secret scan")
	saveCmd.Flags().BoolVar(&saveNoCompress, "no-compress", false, "Skip auto-compression of large stashes")
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
