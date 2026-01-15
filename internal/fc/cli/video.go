package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/client"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/output"
	"github.com/spf13/cobra"
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Video upload and processing commands",
	Long: `Upload and process video files with file.cheap.

Videos are automatically processed for web delivery with adaptive bitrate streaming.

Examples:
  fc video upload movie.mp4                    # Upload a video
  fc video upload movie.mp4 --transcode        # Upload and transcode
  fc video transcode <file-id> -r 720,1080     # Transcode to specific resolutions
  fc video status <file-id>                    # Check processing status`,
}

var videoUploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "Upload a video file",
	Long: `Upload a video file using chunked upload for reliable large file transfers.

Supports: MP4, MOV, AVI, MKV, WebM, WMV, FLV

Thumbnail timestamp can be specified as:
  - Percentage: 50% (of video duration)
  - Duration: 30s, 1m30s, 2m (absolute time)

Examples:
  fc video upload movie.mp4
  fc video upload movie.mp4 --transcode
  fc video upload movie.mp4 --transcode -r 720,1080
  fc video upload movie.mp4 --transcode --thumbnail-at 50%
  fc video upload movie.mp4 --transcode --thumbnail-at 1m30s`,
	Args: cobra.ExactArgs(1),
	RunE: runVideoUpload,
}

var videoTranscodeCmd = &cobra.Command{
	Use:   "transcode <file-id>",
	Short: "Transcode a video to web-optimized formats",
	Long: `Transcode an uploaded video to specified resolutions.

Available resolutions: 360, 480, 720, 1080, 1440, 2160
Available formats: mp4, webm
Available presets: ultrafast, fast, medium, slow

Thumbnail timestamp can be specified as:
  - Percentage: 50% (of video duration)
  - Duration: 30s, 1m30s, 2m (absolute time)

Examples:
  fc video transcode abc123 -r 720
  fc video transcode abc123 -r 720,1080
  fc video transcode abc123 -r 720 -f webm
  fc video transcode abc123 -r 1080 --thumbnail
  fc video transcode abc123 -r 720 --thumbnail-at 2m`,
	Args: cobra.ExactArgs(1),
	RunE: runVideoTranscode,
}

var videoStatusCmd = &cobra.Command{
	Use:   "status <file-id>",
	Short: "Check video processing status",
	Long: `Check the processing status of a video file.

Examples:
  fc video status abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runVideoStatus,
}

var (
	videoTranscode   bool
	videoResolutions []int
	videoFormat      string
	videoPreset      string
	videoThumbnail   bool
	videoThumbnailAt string
	videoWait        bool
)

func init() {
	videoUploadCmd.Flags().BoolVar(&videoTranscode, "transcode", false, "Transcode video after upload")
	videoUploadCmd.Flags().IntSliceVarP(&videoResolutions, "resolution", "r", nil, "Target resolutions (e.g., 720,1080)")
	videoUploadCmd.Flags().StringVarP(&videoFormat, "format", "f", "mp4", "Output format (mp4, webm)")
	videoUploadCmd.Flags().StringVar(&videoPreset, "preset", "medium", "Encoding preset (ultrafast, fast, medium, slow)")
	videoUploadCmd.Flags().BoolVar(&videoThumbnail, "thumbnail", true, "Generate thumbnail")
	videoUploadCmd.Flags().StringVar(&videoThumbnailAt, "thumbnail-at", "", "Thumbnail timestamp (e.g., 30s, 1m30s, 50%)")
	videoUploadCmd.Flags().BoolVarP(&videoWait, "wait", "w", false, "Wait for processing to complete")

	videoTranscodeCmd.Flags().IntSliceVarP(&videoResolutions, "resolution", "r", []int{720}, "Target resolutions")
	videoTranscodeCmd.Flags().StringVarP(&videoFormat, "format", "f", "mp4", "Output format")
	videoTranscodeCmd.Flags().StringVar(&videoPreset, "preset", "medium", "Encoding preset")
	videoTranscodeCmd.Flags().BoolVar(&videoThumbnail, "thumbnail", true, "Generate thumbnail")
	videoTranscodeCmd.Flags().StringVar(&videoThumbnailAt, "thumbnail-at", "", "Thumbnail timestamp (e.g., 30s, 1m30s, 50%)")
	videoTranscodeCmd.Flags().BoolVarP(&videoWait, "wait", "w", false, "Wait for processing")

	videoCmd.AddCommand(videoUploadCmd)
	videoCmd.AddCommand(videoTranscodeCmd)
	videoCmd.AddCommand(videoStatusCmd)
}

func runVideoUpload(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	filePath := args[0]

	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	validExts := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".webm": true, ".wmv": true, ".flv": true,
	}
	if !validExts[ext] {
		return fmt.Errorf("unsupported video format: %s (supported: mp4, mov, avi, mkv, webm, wmv, flv)", ext)
	}

	ctx := GetContext()

	var spinner *output.Spinner
	if !quietMode && !jsonOutput {
		printer.Printf("Uploading %s (%s)...\n", filepath.Base(filePath), formatBytes(stat.Size()))
		spinner = output.NewSpinner("Uploading", quietMode)
	}

	resp, err := apiClient.UploadLargeFile(ctx, filePath, func(uploaded, total int64) {
		if spinner != nil {
			pct := float64(uploaded) / float64(total) * 100
			spinner.Update(fmt.Sprintf("Uploading %.1f%%", pct))
		}
	})

	if spinner != nil {
		spinner.Finish()
	}

	if err != nil {
		if !jsonOutput {
			printer.FileFailed(filePath, err)
		}
		return err
	}

	if !jsonOutput {
		printer.Success("Uploaded %s (ID: %s)", filepath.Base(filePath), resp.ID)
	}

	var transcodeResp *client.VideoTranscodeResponse
	if videoTranscode || len(videoResolutions) > 0 {
		resolutions := videoResolutions
		if len(resolutions) == 0 {
			resolutions = []int{720}
		}

		thumbnailAt, err := parseThumbnailTimestamp(videoThumbnailAt)
		if err != nil {
			return err
		}

		transcodeReq := &client.VideoTranscodeRequest{
			Resolutions: resolutions,
			Format:      videoFormat,
			Preset:      videoPreset,
			Thumbnail:   videoThumbnail,
			ThumbnailAt: thumbnailAt,
		}

		transcodeResp, err = apiClient.VideoTranscode(ctx, resp.ID, transcodeReq)
		if err != nil {
			if !jsonOutput {
				printer.Warn("Upload succeeded but transcode failed: %v", err)
			}
		} else if !jsonOutput {
			printer.Success("Transcode jobs queued: %d", len(transcodeResp.Jobs))
		}
	}

	if videoWait && transcodeResp != nil {
		if !jsonOutput && !quietMode {
			spinner = output.NewSpinner("Processing video...", quietMode)
			file, err := apiClient.WaitForFile(ctx, resp.ID, 5*time.Second, cfg.GetTimeout("upload"))
			spinner.Finish()
			if err != nil {
				printer.Warn("Timeout waiting for processing")
			} else {
				if file.Status == "completed" {
					printer.Success("Video processing completed")
				} else {
					printer.Warn("Video status: %s", file.Status)
				}
			}
		}
	}

	if jsonOutput {
		result := map[string]interface{}{
			"file_id":  resp.ID,
			"filename": resp.Filename,
			"status":   resp.Status,
		}
		if transcodeResp != nil {
			result["transcode_jobs"] = transcodeResp.Jobs
		}
		return printer.JSON(result)
	}

	if !videoWait && (videoTranscode || len(videoResolutions) > 0) {
		printer.Println()
		printer.Printf("Use 'fc video status %s' to check processing progress.\n", resp.ID)
	}

	return nil
}

func runVideoTranscode(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	fileID := args[0]
	ctx := GetContext()

	for _, r := range videoResolutions {
		if r != 360 && r != 480 && r != 720 && r != 1080 && r != 1440 && r != 2160 {
			return fmt.Errorf("invalid resolution: %d (supported: 360, 480, 720, 1080, 1440, 2160)", r)
		}
	}

	if videoFormat != "mp4" && videoFormat != "webm" {
		return fmt.Errorf("invalid format: %s (supported: mp4, webm)", videoFormat)
	}

	validPresets := map[string]bool{
		"ultrafast": true, "fast": true, "medium": true, "slow": true,
	}
	if !validPresets[videoPreset] {
		return fmt.Errorf("invalid preset: %s (supported: ultrafast, fast, medium, slow)", videoPreset)
	}

	thumbnailAt, err := parseThumbnailTimestamp(videoThumbnailAt)
	if err != nil {
		return err
	}

	req := &client.VideoTranscodeRequest{
		Resolutions: videoResolutions,
		Format:      videoFormat,
		Preset:      videoPreset,
		Thumbnail:   videoThumbnail,
		ThumbnailAt: thumbnailAt,
	}

	resp, err := apiClient.VideoTranscode(ctx, fileID, req)
	if err != nil {
		if !jsonOutput {
			printer.FileFailed(fileID, err)
		}
		return err
	}

	if !jsonOutput {
		printer.Success("%s: %d transcode jobs queued", fileID, len(resp.Jobs))
		for _, res := range videoResolutions {
			printer.Printf("  - %dp %s\n", res, videoFormat)
		}
		if videoThumbnail {
			printer.Printf("  - Thumbnail\n")
		}
	}

	if videoWait {
		if !jsonOutput && !quietMode {
			spinner := output.NewSpinner(fmt.Sprintf("Processing %s...", fileID), quietMode)
			file, err := apiClient.WaitForFile(ctx, fileID, 5*time.Second, cfg.GetTimeout("upload"))
			spinner.Finish()
			if err != nil {
				printer.Warn("Timeout waiting for %s", fileID)
			} else {
				if file.Status == "completed" {
					printer.Success("%s processing completed", fileID)
				} else {
					printer.Warn("%s: %s", fileID, file.Status)
				}
			}
		}
	}

	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"file_id":     fileID,
			"jobs":        resp.Jobs,
			"resolutions": videoResolutions,
			"format":      videoFormat,
		})
	}

	if !videoWait {
		printer.Println()
		printer.Printf("Use 'fc video status %s' to check progress.\n", fileID)
	}

	return nil
}

func runVideoStatus(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	fileID := args[0]
	ctx := GetContext()

	file, err := apiClient.GetFile(ctx, fileID)
	if err != nil {
		if !jsonOutput {
			printer.FileFailed(fileID, err)
		}
		return err
	}

	if jsonOutput {
		return printer.JSON(file)
	}

	printer.Printf("File: %s\n", file.Filename)
	printer.Printf("Status: %s\n", file.Status)
	printer.Printf("Size: %s\n", formatBytes(file.SizeBytes))
	printer.Printf("Content-Type: %s\n", file.ContentType)
	printer.Printf("Created: %s\n", file.CreatedAt.Format(time.RFC3339))

	if len(file.Variants) > 0 {
		printer.Println()
		printer.Printf("Variants:\n")
		for _, v := range file.Variants {
			printer.Printf("  - %s: %s (%s)\n", v.VariantType, formatBytes(v.SizeBytes), v.ContentType)
		}
	}

	return nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func parseThumbnailTimestamp(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	// Percentage format: "50%"
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || pct < 0 || pct > 100 {
			return 0, fmt.Errorf("invalid percentage: %s (must be 0-100)", s)
		}
		return pct / 100, nil
	}
	// Duration format: "30s", "1m30s", "1h2m3s"
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s (use 30s, 1m30s, or 50%%)", s)
	}
	// Return negative value to indicate absolute time in seconds
	return -d.Seconds(), nil
}
