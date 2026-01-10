package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/client"
	"github.com/abdul-hamid-achik/file-processor/internal/fc/output"
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:   "upload [files...]",
	Short: "Upload files to file.cheap",
	Long: `Upload one or more files with optional transformations.

Examples:
  fc upload photo.jpg                           # Single file
  fc upload *.jpg                               # Multiple files (glob)
  fc upload photos/ --recursive                 # Directory
  fc upload hero.png -t webp,thumbnail          # With transforms
  fc upload hero.png --name=homepage-hero       # Custom name
  cat screenshot.png | fc upload --stdin -n screenshot.png`,
	RunE: runUpload,
}

var (
	uploadTransforms []string
	uploadName       string
	uploadRecursive  bool
	uploadParallel   int
	uploadStdin      bool
	uploadDryRun     bool
	uploadWait       bool
)

func init() {
	uploadCmd.Flags().StringSliceVarP(&uploadTransforms, "transform", "t", nil, "Transforms to apply (comma-separated)")
	uploadCmd.Flags().StringVarP(&uploadName, "name", "n", "", "Custom filename")
	uploadCmd.Flags().BoolVarP(&uploadRecursive, "recursive", "r", false, "Process directories recursively")
	uploadCmd.Flags().IntVarP(&uploadParallel, "parallel", "p", 0, "Parallel uploads (default: 4)")
	uploadCmd.Flags().BoolVar(&uploadStdin, "stdin", false, "Read from stdin")
	uploadCmd.Flags().BoolVar(&uploadDryRun, "dry-run", false, "Show what would be uploaded")
	uploadCmd.Flags().BoolVarP(&uploadWait, "wait", "w", false, "Wait for processing to complete")
}

func runUpload(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	parallel := uploadParallel
	if parallel == 0 {
		parallel = cfg.Parallel
	}

	transforms := uploadTransforms
	if len(transforms) == 0 && len(cfg.DefaultTransforms) > 0 {
		transforms = cfg.DefaultTransforms
	}

	if uploadStdin {
		return uploadFromStdin(transforms)
	}

	if len(args) == 0 {
		return fmt.Errorf("no files specified")
	}

	files, err := collectFiles(args, uploadRecursive)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no files to upload")
	}

	if uploadDryRun {
		return printDryRun(files, transforms)
	}

	return uploadFiles(files, transforms, parallel)
}

func uploadFromStdin(transforms []string) error {
	if uploadName == "" {
		return fmt.Errorf("--name is required when using --stdin")
	}

	printer.Info("Reading from stdin...")

	tmpFile, err := os.CreateTemp("", "fc-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	size, err := io.Copy(tmpFile, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek temp file: %w", err)
	}

	ctx := GetContext()
	result, err := apiClient.UploadReader(ctx, tmpFile, uploadName, size, transforms, uploadWait)
	if err != nil {
		printer.FileFailed(uploadName, err)
		if jsonOutput {
			return printer.JSON(client.UploadSummary{
				Failed:     []client.UploadResult{{File: uploadName, Error: err}},
				Total:      1,
				Successful: 0,
			})
		}
		return err
	}

	if uploadWait && !jsonOutput {
		spinner := output.NewSpinner("Waiting for processing...", quietMode)
		file, err := apiClient.WaitForFile(ctx, result.ID, 2*time.Second, cfg.GetTimeout("upload"))
		spinner.Finish()
		if err != nil {
			printer.Warn("Processing status unknown: %v", err)
		} else if file.Status == "failed" {
			printer.Warn("Processing failed")
		}
	}

	if jsonOutput {
		return printer.JSON(client.UploadSummary{
			Uploaded: []client.UploadResult{{
				File:     uploadName,
				FileID:   result.ID,
				URL:      result.URL,
				Variants: result.Variants,
				Status:   result.Status,
			}},
			Total:      1,
			Successful: 1,
		})
	}

	printer.FileUploaded(uploadName, result.URL, result.Variants)
	printer.Summary(1, 0)
	return nil
}

func collectFiles(args []string, recursive bool) ([]string, error) {
	var files []string

	for _, arg := range args {
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
		}

		if len(matches) == 0 {
			if _, err := os.Stat(arg); err != nil {
				return nil, fmt.Errorf("file not found: %s", arg)
			}
			matches = []string{arg}
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}

			if info.IsDir() {
				if recursive {
					err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return nil
						}
						if !info.IsDir() && isImageFile(path) {
							files = append(files, path)
						}
						return nil
					})
					if err != nil {
						return nil, err
					}
				}
			} else if isImageFile(match) {
				files = append(files, match)
			}
		}
	}

	return files, nil
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".tiff", ".tif":
		return true
	}
	return false
}

func printDryRun(files []string, transforms []string) error {
	printer.Section("Dry Run - Would upload:")
	printer.Println()

	for _, f := range files {
		printer.Printf("  %s\n", f)
	}

	printer.Println()
	printer.Printf("Total: %d files\n", len(files))

	if len(transforms) > 0 {
		printer.Printf("Transforms: %s\n", strings.Join(transforms, ", "))
	}

	return nil
}

func uploadFiles(files []string, transforms []string, parallel int) error {
	ctx := GetContext()

	if !quietMode && !jsonOutput {
		printer.Printf("Uploading %d files...\n", len(files))
	}

	results := make(chan client.UploadResult, len(files))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup

	progress := output.NewProgress(len(files), "Uploading", output.ProgressWithQuiet(quietMode || jsonOutput))

	for _, file := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := apiClient.Upload(ctx, f, transforms, uploadWait)
			if err != nil {
				results <- client.UploadResult{File: f, Error: err}
			} else {
				results <- client.UploadResult{
					File:     f,
					FileID:   result.ID,
					URL:      result.URL,
					Variants: result.Variants,
					Status:   result.Status,
				}
			}
			progress.Increment()
		}(file)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var uploaded, failed []client.UploadResult
	for result := range results {
		if result.Error != nil {
			failed = append(failed, result)
			if !jsonOutput {
				printer.FileFailed(result.File, result.Error)
			}
		} else {
			uploaded = append(uploaded, result)
			if !jsonOutput && !quietMode {
				printer.FileUploaded(result.File, result.URL, result.Variants)
			}
		}
	}

	progress.Finish()

	if jsonOutput {
		return printer.JSON(client.UploadSummary{
			Uploaded:   uploaded,
			Failed:     failed,
			Total:      len(files),
			Successful: len(uploaded),
		})
	}

	printer.Summary(len(uploaded), len(failed))

	if len(failed) > 0 {
		return fmt.Errorf("%d uploads failed", len(failed))
	}
	return nil
}
