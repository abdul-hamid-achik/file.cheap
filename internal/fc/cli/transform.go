package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/client"
	"github.com/abdul-hamid-achik/file-processor/internal/fc/output"
	"github.com/spf13/cobra"
)

var transformCmd = &cobra.Command{
	Use:   "transform [file-id...]",
	Short: "Apply transformations to existing files",
	Long: `Apply transformations to files already uploaded to file.cheap.

Examples:
  fc transform abc123 -t webp,thumbnail
  fc transform abc123 def456 -t resize:lg
  fc transform abc123 --preset=social
  fc transform abc123 -t watermark:"© 2026" --wait`,
	RunE: runTransform,
}

var (
	transformPresets   []string
	transformTransform []string
	transformQuality   int
	transformWatermark string
	transformWait      bool
)

func init() {
	transformCmd.Flags().StringSliceVarP(&transformPresets, "preset", "p", nil, "Presets to apply")
	transformCmd.Flags().StringSliceVarP(&transformTransform, "transform", "t", nil, "Transforms to apply")
	transformCmd.Flags().IntVarP(&transformQuality, "quality", "q", 0, "Quality (1-100)")
	transformCmd.Flags().StringVar(&transformWatermark, "watermark", "", "Watermark text")
	transformCmd.Flags().BoolVarP(&transformWait, "wait", "w", false, "Wait for processing")
}

func runTransform(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("no file IDs specified")
	}

	presets := transformPresets
	for _, t := range transformTransform {
		presets = append(presets, t)
	}

	if len(presets) == 0 && transformWatermark == "" {
		return fmt.Errorf("no transforms specified (use -t or -p)")
	}

	ctx := context.Background()
	var successful, failed int
	var results []map[string]interface{}

	for _, fileID := range args {
		req := &client.TransformRequest{
			Presets:   presets,
			Quality:   transformQuality,
			Watermark: transformWatermark,
		}

		resp, err := apiClient.Transform(ctx, fileID, req)
		if err != nil {
			if !jsonOutput {
				printer.FileFailed(fileID, err)
			}
			results = append(results, map[string]interface{}{
				"file_id": fileID,
				"error":   err.Error(),
			})
			failed++
			continue
		}

		if !jsonOutput {
			printer.Success("%s: %d jobs queued", fileID, len(resp.Jobs))
		}

		result := map[string]interface{}{
			"file_id": fileID,
			"jobs":    resp.Jobs,
		}

		if transformWait {
			if !jsonOutput && !quietMode {
				spinner := output.NewSpinner(fmt.Sprintf("Waiting for %s...", fileID), quietMode)
				file, err := apiClient.WaitForFile(ctx, fileID, 2*time.Second, 5*time.Minute)
				spinner.Finish()
				if err != nil {
					printer.Warn("Timeout waiting for %s", fileID)
					result["status"] = "timeout"
				} else {
					result["status"] = file.Status
					if file.Status == "completed" {
						printer.Success("%s completed", fileID)
					} else {
						printer.Warn("%s: %s", fileID, file.Status)
					}
				}
			}
		}

		results = append(results, result)
		successful++
	}

	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"results":    results,
			"total":      len(args),
			"successful": successful,
			"failed":     failed,
		})
	}

	if !transformWait {
		printer.Println()
		printer.Printf("Transforms queued. Use 'fc status <file-id>' to check progress.\n")
	}

	printer.Summary(successful, failed)

	if failed > 0 {
		return fmt.Errorf("%d transforms failed", failed)
	}
	return nil
}

func parseTransformString(t string) (string, string) {
	parts := strings.SplitN(t, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return t, ""
}
