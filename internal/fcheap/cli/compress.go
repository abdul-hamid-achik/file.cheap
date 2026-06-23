package cli

import (
	"fmt"

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

		algo := compressAlgo
		if algo == "" {
			algo = cfg.Compression
		}

		res, err := mgr.Compress(GetContext(), args[0], algo)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(res)
		}

		printer.Success("Compressed stash: %s", res.ID)
		printer.KeyValue("Algorithm", res.Algorithm)
		printer.KeyValue("Archive", res.ArchivePath)
		printer.KeyValue("Original Size", formatSize(res.OriginalSize))
		printer.KeyValue("Compressed Size", formatSize(res.CompressedSize))
		if res.OriginalSize > 0 {
			ratio := 100.0 * (1.0 - float64(res.CompressedSize)/float64(res.OriginalSize))
			printer.KeyValue("Saved", fmt.Sprintf("%.0f%%", ratio))
		}
		return nil
	},
}

func init() {
	compressCmd.Flags().StringVar(&compressAlgo, "algo", "", "Compression algorithm: zstd (default), gzip, none")
}
