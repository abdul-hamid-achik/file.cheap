package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	saveName       string
	saveTags       []string
	saveTool       string
	saveSource     string
	saveTTL        string
	saveNoScan     bool
	saveNoCompress bool
	saveIndex      bool
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

		// Auto-apply TTL from config when --ttl was not explicitly set.
		appliedTTL := saveTTL
		ttlFromConfig := ""
		if !cmd.Flags().Changed("ttl") {
			if v := cfg.TTLForTool(saveTool); v != "" {
				appliedTTL = v
				ttlFromConfig = v
			}
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
			TTL:        appliedTTL,
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

		// Optionally index the stash for search right after saving, so callers
		// (e.g. Cortex) that save evidence can search it without a separate
		// `fcheap analyze` step. Best-effort: a save that succeeds is never failed
		// by an indexing error.
		var indexed *analyze.IndexResult
		if saveIndex {
			an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).WithEmbedder(embSettings())
			indexed, err = an.IndexStash(GetContext(), mgr.StashDir(st.Manifest.ID))
			if err != nil {
				printer.Warn("index after save failed: %v (run 'fcheap analyze %s')", err, st.Manifest.ID)
				err = nil
			} else if st.Manifest.Custom == nil {
				st.Manifest.Custom = map[string]string{}
			}
			if indexed != nil {
				st.Manifest.Custom["indexed"] = "true"
				st.Manifest.Custom["indexed_files"] = fmt.Sprintf("%d", indexed.FilesIndex)
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
		if st.Manifest.ExpiresAt != "" {
			printer.KeyValue("Expires", st.Manifest.ExpiresAt)
			if ttlFromConfig != "" {
				printer.KeyValue("TTL source", fmt.Sprintf("config (%s)", describeTTLSource(saveTool, ttlFromConfig, cfg)))
			}
		}
		if autoCompressed {
			printer.KeyValue("Compressed", fmt.Sprintf("%s → %s (%s)", formatSize(st.Manifest.TotalSize), formatSize(st.Manifest.CompressedSize), st.Manifest.Compression))
		}
		if indexed != nil {
			printer.KeyValue("Indexed", fmt.Sprintf("%d file(s) [%s]", indexed.FilesIndex, indexed.BundleType))
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
	saveCmd.Flags().StringVar(&saveTTL, "ttl", "", "Time-to-live for this stash (e.g. 7d, 24h, 30d); empty = never expires")
	saveCmd.Flags().BoolVar(&saveNoScan, "no-scan", false, "Skip the save-time secret scan")
	saveCmd.Flags().BoolVar(&saveNoCompress, "no-compress", false, "Skip auto-compression of large stashes")
	saveCmd.Flags().BoolVar(&saveIndex, "index", false, "Index the stash for search immediately after saving (so it's searchable without a separate 'fcheap analyze' step)")
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

// describeTTLSource returns a human-readable label for which config key
// provided the auto-applied TTL. It distinguishes between a per-tool rule
// (ttl_rules.<tool>) and the default_ttl fallback.
func describeTTLSource(tool, ttl string, cfg *config.Config) string {
	if tool != "" {
		if v, ok := cfg.TTLRules[tool]; ok && normalizeTTL(v) == ttl {
			return fmt.Sprintf("ttl_rules.%s=%s", tool, v)
		}
	}
	return fmt.Sprintf("default_ttl=%s", cfg.DefaultTTL)
}

// normalizeTTL converts "never" to "" so it can be compared with the
// already-normalized value returned by TTLForTool.
func normalizeTTL(v string) string {
	if v == "never" {
		return ""
	}
	return v
}
