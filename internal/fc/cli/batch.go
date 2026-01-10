package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/client"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/output"
	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch [directory]",
	Short: "Batch process files",
	Long: `Batch process multiple files with presets or custom transforms.

Examples:
  fc batch ./products --preset=ecommerce
  fc batch ./photos -t webp,resize:lg
  fc batch --ids=abc123,def456 -t thumbnail
  fc batch --file=images.txt -t webp
  fc batch ./large-folder --preset=social --progress`,
	RunE: runBatch,
}

var (
	batchPreset    string
	batchTransform []string
	batchIDs       string
	batchFile      string
	batchQuality   int
	batchWatermark string
	batchProgress  bool
	batchWait      bool
)

func init() {
	batchCmd.Flags().StringVarP(&batchPreset, "preset", "p", "", "Preset to apply (ecommerce, social, blog, avatar, responsive)")
	batchCmd.Flags().StringSliceVarP(&batchTransform, "transform", "t", nil, "Transforms to apply")
	batchCmd.Flags().StringVar(&batchIDs, "ids", "", "Comma-separated file IDs to process")
	batchCmd.Flags().StringVar(&batchFile, "file", "", "File containing list of file IDs (one per line)")
	batchCmd.Flags().IntVarP(&batchQuality, "quality", "q", 0, "Quality (1-100)")
	batchCmd.Flags().StringVar(&batchWatermark, "watermark", "", "Watermark text")
	batchCmd.Flags().BoolVar(&batchProgress, "progress", false, "Show progress bar")
	batchCmd.Flags().BoolVarP(&batchWait, "wait", "w", false, "Wait for batch to complete")
}

func runBatch(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	var fileIDs []string
	var err error

	if batchIDs != "" {
		fileIDs = strings.Split(batchIDs, ",")
		for i, id := range fileIDs {
			fileIDs[i] = strings.TrimSpace(id)
		}
	} else if batchFile != "" {
		fileIDs, err = readFileIDs(batchFile)
		if err != nil {
			return err
		}
	} else if len(args) > 0 {
		files, err := collectFiles(args, true)
		if err != nil {
			return err
		}
		return uploadAndBatchProcess(files)
	} else {
		return fmt.Errorf("specify file IDs with --ids, --file, or provide a directory")
	}

	if len(fileIDs) == 0 {
		return fmt.Errorf("no file IDs to process")
	}

	return batchProcessIDs(fileIDs)
}

func readFileIDs(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var ids []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" && !strings.HasPrefix(id, "#") {
			ids = append(ids, id)
		}
	}

	return ids, scanner.Err()
}

func batchProcessIDs(fileIDs []string) error {
	ctx := GetContext()

	presets := batchTransform
	if batchPreset != "" {
		preset, ok := cfg.GetPreset(batchPreset)
		if !ok {
			return fmt.Errorf("unknown preset: %s", batchPreset)
		}
		presets = append(presets, preset.Transforms...)
	}

	if len(presets) == 0 && batchWatermark == "" {
		return fmt.Errorf("no transforms specified (use -t, -p, or --watermark)")
	}

	printer.Printf("Processing %d files...\n", len(fileIDs))

	req := &client.BatchTransformRequest{
		FileIDs:   fileIDs,
		Presets:   presets,
		Quality:   batchQuality,
		Watermark: batchWatermark,
	}

	resp, err := apiClient.BatchTransform(ctx, req)
	if err != nil {
		return fmt.Errorf("batch transform failed: %w", err)
	}

	if jsonOutput && !batchWait {
		return printer.JSON(resp)
	}

	printer.Success("Batch %s created: %d files, %d jobs", resp.BatchID, resp.TotalFiles, resp.TotalJobs)

	if batchWait || batchProgress {
		return waitForBatch(ctx, resp.BatchID)
	}

	printer.Info("Use 'fc status --batch=%s' to check progress", resp.BatchID)
	return nil
}

func waitForBatch(ctx context.Context, batchID string) error {
	printer.Printf("Waiting for batch to complete...\n")

	spinner := output.NewSpinner("Processing", quietMode || jsonOutput)
	defer spinner.Finish()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(cfg.GetTimeout("batch_wait"))
	var consecutiveErrors int
	const maxConsecutiveErrors = 5

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("batch timed out")
		case <-ticker.C:
			status, err := apiClient.GetBatchStatus(ctx, batchID, true)
			if err != nil {
				consecutiveErrors++

				// Check for fatal auth errors
				if isAuthError(err) {
					return fmt.Errorf("authentication failed: %w", err)
				}

				// Update spinner to show error state
				spinner.Update(fmt.Sprintf("Processing: error (%d/%d retries)", consecutiveErrors, maxConsecutiveErrors))

				// Give up after too many consecutive errors
				if consecutiveErrors >= maxConsecutiveErrors {
					return fmt.Errorf("failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				continue
			}

			// Reset error count on success
			consecutiveErrors = 0
			spinner.Update(fmt.Sprintf("Processing: %d/%d completed", status.CompletedFiles, status.TotalFiles))

			if status.Status == "completed" || status.Status == "failed" || status.Status == "partial" {
				spinner.Finish()

				if jsonOutput {
					return printer.JSON(status)
				}

				printer.Println()
				switch status.Status {
				case "completed":
					printer.Success("Batch completed: %d files processed", status.CompletedFiles)
				case "partial":
					printer.Warn("Batch partially completed: %d succeeded, %d failed", status.CompletedFiles, status.FailedFiles)
				default:
					printer.Error("Batch failed: %d files failed", status.FailedFiles)
				}

				return nil
			}
		}
	}
}

func uploadAndBatchProcess(files []string) error {
	ctx := GetContext()

	printer.Printf("Uploading %d files...\n", len(files))

	var fileIDs []string
	progress := output.NewProgress(len(files), "Uploading", output.ProgressWithQuiet(quietMode || jsonOutput))

	for _, file := range files {
		result, err := apiClient.Upload(ctx, file, nil, false)
		if err != nil {
			printer.FileFailed(file, err)
			continue
		}
		fileIDs = append(fileIDs, result.ID)
		progress.Increment()
	}

	progress.Finish()

	if len(fileIDs) == 0 {
		return fmt.Errorf("no files uploaded successfully")
	}

	printer.Success("Uploaded %d files", len(fileIDs))
	printer.Println()

	return batchProcessIDs(fileIDs)
}
