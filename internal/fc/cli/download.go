package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// sanitizeFilename removes path traversal attempts and invalid characters from a filename.
// It returns an empty string if the filename is invalid or dangerous.
func sanitizeFilename(filename string) string {
	// Take only the base name, removing any path components
	filename = filepath.Base(filename)

	// Clean the path to remove . and .. components
	filename = filepath.Clean(filename)

	// Reject if it still looks dangerous
	if filename == "." || filename == ".." || filename == "" || filename == "/" {
		return ""
	}

	// Remove null bytes and path separators
	filename = strings.Map(func(r rune) rune {
		if r == 0 || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, filename)

	return filename
}

// safePath ensures the final path stays within the target directory.
// Returns an error if the path would escape the base directory.
func safePath(baseDir, filename string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}

	sanitized := sanitizeFilename(filename)
	if sanitized == "" {
		return "", fmt.Errorf("invalid filename")
	}

	fullPath := filepath.Join(absBase, sanitized)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// Ensure the path is still within baseDir
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", fmt.Errorf("path escapes target directory")
	}

	return absPath, nil
}

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

	ctx := GetContext()
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

	// Sanitize the filename to prevent path traversal attacks
	filename = sanitizeFilename(filename)
	if filename == "" {
		return fmt.Errorf("server returned invalid filename")
	}

	var outputPath string
	info, err := os.Stat(downloadOutput)
	if err == nil && info.IsDir() {
		// Output is a directory - use safePath to construct the full path
		outputPath, err = safePath(downloadOutput, filename)
		if err != nil {
			return fmt.Errorf("unsafe download path: %w", err)
		}
	} else if os.IsNotExist(err) {
		// Output path doesn't exist - treat it as a file path
		dir := filepath.Dir(downloadOutput)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}
		outputPath = downloadOutput
	} else {
		// Output exists and is a file - use it directly
		outputPath = downloadOutput
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
