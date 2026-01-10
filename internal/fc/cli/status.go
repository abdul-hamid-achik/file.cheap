package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/output"
	"github.com/spf13/cobra"
)

const maxConsecutiveErrors = 5

// isAuthError checks if an error indicates an authentication failure
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "Unauthorized")
}

var statusCmd = &cobra.Command{
	Use:   "status [file-id]",
	Short: "Check processing job status",
	Long: `Check the status of a file or batch operation.

Examples:
  fc status abc123              # Check file status
  fc status --batch=batch_xyz   # Check batch status
  fc status abc123 --watch      # Watch until complete`,
	RunE: runStatus,
}

var (
	statusBatch string
	statusWatch bool
)

func init() {
	statusCmd.Flags().StringVar(&statusBatch, "batch", "", "Batch ID to check")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Watch until complete")
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	ctx := GetContext()

	if statusBatch != "" {
		return checkBatchStatus(ctx, statusBatch)
	}

	if len(args) == 0 {
		return fmt.Errorf("specify a file ID or use --batch")
	}

	return checkFileStatus(ctx, args[0])
}

func checkFileStatus(ctx context.Context, fileID string) error {
	if statusWatch {
		return watchFileStatus(ctx, fileID)
	}

	file, err := apiClient.GetFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get file status: %w", err)
	}

	if jsonOutput {
		return printer.JSON(file)
	}

	printer.Section("File Status")
	printer.KeyValue("ID", file.ID)
	printer.KeyValue("Filename", file.Filename)
	printer.KeyValue("Status", file.Status)
	printer.KeyValue("Size", formatSize(file.SizeBytes))
	printer.KeyValue("Created", formatTime(file.CreatedAt))

	if len(file.Variants) > 0 {
		printer.Section("Variants")
		table := output.NewTable([]string{"Type", "Size", "Dimensions"}, quietMode)
		for _, v := range file.Variants {
			dims := ""
			if v.Width > 0 && v.Height > 0 {
				dims = fmt.Sprintf("%dx%d", v.Width, v.Height)
			}
			table.Append([]string{v.VariantType, formatSize(v.SizeBytes), dims})
		}
		table.Render()
	}

	return nil
}

func watchFileStatus(ctx context.Context, fileID string) error {
	spinner := output.NewSpinner(fmt.Sprintf("Watching %s...", fileID), quietMode || jsonOutput)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(cfg.GetTimeout("status_watch"))
	var consecutiveErrors int

	for {
		select {
		case <-ctx.Done():
			spinner.Finish()
			return ctx.Err()
		case <-timeout:
			spinner.Finish()
			return fmt.Errorf("timed out waiting for file")
		case <-ticker.C:
			file, err := apiClient.GetFile(ctx, fileID)
			if err != nil {
				consecutiveErrors++

				// Check for fatal auth errors
				if isAuthError(err) {
					spinner.Finish()
					return fmt.Errorf("authentication failed: %w", err)
				}

				// Update spinner to show error state
				spinner.Update(fmt.Sprintf("Status: error (%d/%d retries)", consecutiveErrors, maxConsecutiveErrors))

				// Give up after too many consecutive errors
				if consecutiveErrors >= maxConsecutiveErrors {
					spinner.Finish()
					return fmt.Errorf("failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				continue
			}

			// Reset error count on success
			consecutiveErrors = 0
			spinner.Update(fmt.Sprintf("Status: %s", file.Status))

			if file.Status == "completed" || file.Status == "failed" {
				spinner.Finish()

				if jsonOutput {
					return printer.JSON(file)
				}

				if file.Status == "completed" {
					printer.Success("File %s completed", fileID)
					if len(file.Variants) > 0 {
						printer.Section("Variants")
						for _, v := range file.Variants {
							printer.Printf("  %s: %dx%d\n", v.VariantType, v.Width, v.Height)
						}
					}
				} else {
					printer.Error("File %s failed", fileID)
				}
				return nil
			}
		}
	}
}

func checkBatchStatus(ctx context.Context, batchID string) error {
	if statusWatch {
		return watchBatchStatus(ctx, batchID)
	}

	status, err := apiClient.GetBatchStatus(ctx, batchID, true)
	if err != nil {
		return fmt.Errorf("failed to get batch status: %w", err)
	}

	if jsonOutput {
		return printer.JSON(status)
	}

	printer.Section("Batch Status")
	printer.KeyValue("ID", status.ID)
	printer.KeyValue("Status", status.Status)
	printer.KeyValue("Total Files", fmt.Sprintf("%d", status.TotalFiles))
	printer.KeyValue("Completed", fmt.Sprintf("%d", status.CompletedFiles))
	printer.KeyValue("Failed", fmt.Sprintf("%d", status.FailedFiles))
	printer.KeyValue("Created", formatTime(status.CreatedAt))

	if len(status.Presets) > 0 {
		printer.KeyValue("Presets", fmt.Sprintf("%v", status.Presets))
	}

	if len(status.Items) > 0 {
		printer.Section("Items")
		table := output.NewTable([]string{"File ID", "Status", "Jobs"}, quietMode)
		for _, item := range status.Items {
			table.Append([]string{
				item.FileID[:8] + "...",
				item.Status,
				fmt.Sprintf("%d", len(item.JobIDs)),
			})
		}
		table.Render()
	}

	return nil
}

func watchBatchStatus(ctx context.Context, batchID string) error {
	spinner := output.NewSpinner(fmt.Sprintf("Watching batch %s...", batchID), quietMode || jsonOutput)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(cfg.GetTimeout("batch_wait"))
	var consecutiveErrors int

	for {
		select {
		case <-ctx.Done():
			spinner.Finish()
			return ctx.Err()
		case <-timeout:
			spinner.Finish()
			return fmt.Errorf("timed out waiting for batch")
		case <-ticker.C:
			status, err := apiClient.GetBatchStatus(ctx, batchID, false)
			if err != nil {
				consecutiveErrors++

				// Check for fatal auth errors
				if isAuthError(err) {
					spinner.Finish()
					return fmt.Errorf("authentication failed: %w", err)
				}

				// Update spinner to show error state
				spinner.Update(fmt.Sprintf("Status: error (%d/%d retries)", consecutiveErrors, maxConsecutiveErrors))

				// Give up after too many consecutive errors
				if consecutiveErrors >= maxConsecutiveErrors {
					spinner.Finish()
					return fmt.Errorf("failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				continue
			}

			// Reset error count on success
			consecutiveErrors = 0
			spinner.Update(fmt.Sprintf("%d/%d completed", status.CompletedFiles, status.TotalFiles))

			if status.Status == "completed" || status.Status == "failed" || status.Status == "partial" {
				spinner.Finish()

				if jsonOutput {
					fullStatus, _ := apiClient.GetBatchStatus(ctx, batchID, true)
					return printer.JSON(fullStatus)
				}

				switch status.Status {
				case "completed":
					printer.Success("Batch completed: %d files", status.CompletedFiles)
				case "partial":
					printer.Warn("Batch partial: %d succeeded, %d failed", status.CompletedFiles, status.FailedFiles)
				default:
					printer.Error("Batch failed: %d files failed", status.FailedFiles)
				}
				return nil
			}
		}
	}
}
