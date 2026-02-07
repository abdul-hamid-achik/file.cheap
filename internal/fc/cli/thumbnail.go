package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var (
	thumbWidth    int
	thumbHeight   int
	thumbPosition string
)

var thumbnailCmd = &cobra.Command{
	Use:   "thumbnail <files...>",
	Short: "Generate thumbnails",
	Long: `Generate thumbnails from images, videos, or PDFs.
File type is auto-detected.

Examples:
  fc thumbnail photo.jpg
  fc thumbnail *.jpg --width 200 --height 200
  fc thumbnail video.mp4
  fc thumbnail document.pdf`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var expanded []string
		for _, f := range args {
			matches, err := filepath.Glob(f)
			if err != nil || len(matches) == 0 {
				expanded = append(expanded, f)
			} else {
				expanded = append(expanded, matches...)
			}
		}

		reqs := make([]*engine.Request, len(expanded))
		for i, file := range expanded {
			procName := detectThumbnailProcessor(file)
			req := &engine.Request{
				InputPath: file,
				Processor: procName,
				Options: &processor.Options{
					Width:    thumbWidth,
					Height:   thumbHeight,
					Quality:  cfg.Quality,
					Position: thumbPosition,
				},
			}
			if cfg.OutputDir != "" {
				base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
				req.OutputPath = filepath.Join(cfg.OutputDir, base+"_thumb.jpg")
			}
			reqs[i] = req
		}

		results, errs := eng.ProcessBatch(GetContext(), reqs, cfg.EffectiveParallel())

		successful := 0
		failed := 0
		for i, res := range results {
			if errs[i] != nil {
				printer.FileFailed(expanded[i], errs[i])
				failed++
				continue
			}
			successful++
			printer.Success("%s -> %s (%s)", expanded[i], res.OutputPath, formatSize(res.OutputSize))
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{"successful": successful, "failed": failed, "results": results})
		}
		if len(expanded) > 1 {
			printer.Summary(successful, failed)
		}
		if failed > 0 {
			return fmt.Errorf("%d file(s) failed", failed)
		}
		return nil
	},
}

func init() {
	thumbnailCmd.Flags().IntVar(&thumbWidth, "width", 300, "Thumbnail width")
	thumbnailCmd.Flags().IntVar(&thumbHeight, "height", 300, "Thumbnail height")
	thumbnailCmd.Flags().StringVar(&thumbPosition, "position", "center", "Crop position (center, north, south, east, west)")
}

func detectThumbnailProcessor(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "pdf_thumbnail"
	case ".mp4", ".webm", ".mov", ".avi", ".mkv", ".mpeg", ".mpg", ".ogv", ".3gp":
		return "video_thumbnail"
	default:
		return "thumbnail"
	}
}
