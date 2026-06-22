package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/compress"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var compressAlgo string

var compressCmd = &cobra.Command{
	Use:   "compress <stash-id>",
	Short: "Compress a stash to save space",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if !mgr.Exists(args[0]) {
			return fmt.Errorf("stash not found: %s", args[0])
		}

		stashDir := mgr.StashDir(args[0])
		contentDir := filepath.Join(stashDir, "content")
		archivePath := filepath.Join(stashDir, "content.tar.zst")

		algo := compress.Algorithm(compressAlgo)
		if algo == "" {
			algo = compress.Algorithm(cfg.Compression)
		}

		if algo == compress.Zstd {
			archivePath = filepath.Join(stashDir, "content.tar.zst")
		} else if algo == compress.Gzip {
			archivePath = filepath.Join(stashDir, "content.tar.gz")
		} else {
			archivePath = filepath.Join(stashDir, "content.tar")
		}

		compressedSize, err := compress.Archive(contentDir, archivePath, algo)
		if err != nil {
			return err
		}

		// Optionally remove extracted content after compression
		// (keep for now — user can drop manually)

		if printer.IsJSON() {
			return printer.JSON(map[string]any{
				"stash_id":        args[0],
				"algorithm":       string(algo),
				"archive_path":    archivePath,
				"compressed_size": compressedSize,
			})
		}

		printer.Success("Compressed stash: %s", args[0])
		printer.KeyValue("Algorithm", string(algo))
		printer.KeyValue("Archive", archivePath)
		printer.KeyValue("Compressed Size", formatSize(compressedSize))
		return nil
	},
}

func init() {
	compressCmd.Flags().StringVar(&compressAlgo, "algo", "", "Compression algorithm: zstd (default), gzip, none")
}