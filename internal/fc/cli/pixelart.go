package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var (
	pixelSize   int
	pixelFormat string
)

var pixelartCmd = &cobra.Command{
	Use:   "pixelart <files...>",
	Short: "Convert images to pixel art",
	Long: `Convert images into pixel art by reducing and re-upscaling with a configurable block size.
Uses Go standard libraries only — no external tools required.

Examples:
  fc pixelart photo.jpg
  fc pixelart photo.jpg --pixel-size 8
  fc pixelart *.png --pixel-size 32 --format jpeg
  fc pixelart photo.jpg --output-dir ./out`,
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
			req := &engine.Request{
				InputPath: file,
				Processor: "pixelart",
				Options: &processor.Options{
					Width:  pixelSize,
					Format: pixelFormat,
				},
			}
			if cfg.OutputDir != "" {
				baseName := filepath.Base(file)
				ext := filepath.Ext(baseName)
				nameOnly := baseName[:len(baseName)-len(ext)]
				format := pixelFormat
				if format == "" {
					format = "png"
				}
				req.OutputPath = filepath.Join(cfg.OutputDir, nameOnly+"_pixelart."+format)
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
			printer.Success("%s -> %s", expanded[i], res.OutputPath)
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{
				"successful": successful,
				"failed":     failed,
				"results":    results,
			})
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
	pixelartCmd.Flags().IntVar(&pixelSize, "pixel-size", 16, "Size of each pixel block")
	pixelartCmd.Flags().StringVar(&pixelFormat, "format", "png", "Output format (png or jpeg)")
}
