package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:   "download [file-id...]",
	Short: "Download files or variants",
	Long: `Download files from file.cheap.

Examples:
  fc download abc123                  # Download original
  fc download abc123 --variant=thumbnail  # Download variant
  fc download abc123 -o ./downloads/  # To specific path
  fc download abc123 def456 -o ./backup/  # Multiple files
  fc download abc123 --all-variants   # Download all variants`,
	RunE: runDownload,
}

var (
	downloadOutput      string
	downloadVariant     string
	downloadAllVariants bool
)

func init() {
	downloadCmd.Flags().StringVarP(&downloadOutput, "output", "o", ".", "Output directory or file path")
	downloadCmd.Flags().StringVar(&downloadVariant, "variant", "", "Download specific variant")
	downloadCmd.Flags().BoolVar(&downloadAllVariants, "all-variants", false, "Download all variants")
}

func runDownload(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("no file IDs specified")
	}

	ctx := context.Background()
	var successful, failed int

	for _, fileID := range args {
		if downloadAllVariants {
			if err := downloadFileWithVariants(ctx, fileID); err != nil {
				printer.FileFailed(fileID, err)
				failed++
			} else {
				successful++
			}
		} else {
			if err := downloadSingleFile(ctx, fileID, downloadVariant); err != nil {
				printer.FileFailed(fileID, err)
				failed++
			} else {
				successful++
			}
		}
	}

	if !jsonOutput {
		printer.Summary(successful, failed)
	}

	if failed > 0 {
		return fmt.Errorf("%d downloads failed", failed)
	}
	return nil
}

func downloadSingleFile(ctx context.Context, fileID, variant string) error {
	body, filename, err := apiClient.Download(ctx, fileID, variant)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	if filename == "" {
		file, err := apiClient.GetFile(ctx, fileID)
		if err != nil {
			filename = fileID
		} else {
			filename = file.Filename
		}
		if variant != "" {
			ext := filepath.Ext(filename)
			base := filename[:len(filename)-len(ext)]
			filename = fmt.Sprintf("%s_%s%s", base, variant, ext)
		}
	}

	outputPath := downloadOutput
	info, err := os.Stat(outputPath)
	if err == nil && info.IsDir() {
		outputPath = filepath.Join(outputPath, filename)
	} else if os.IsNotExist(err) {
		dir := filepath.Dir(outputPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	_, err = io.Copy(outFile, body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	printer.Success("Downloaded %s to %s", filename, outputPath)
	return nil
}

func downloadFileWithVariants(ctx context.Context, fileID string) error {
	file, err := apiClient.GetFile(ctx, fileID)
	if err != nil {
		return err
	}

	if err := downloadSingleFile(ctx, fileID, ""); err != nil {
		return err
	}

	for _, variant := range file.Variants {
		if err := downloadSingleFile(ctx, fileID, variant.VariantType); err != nil {
			printer.Warn("Failed to download variant %s: %v", variant.VariantType, err)
		}
	}

	return nil
}
