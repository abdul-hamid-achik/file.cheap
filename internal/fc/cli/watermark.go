package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var (
	wmPosition string
	wmOpacity  int
	wmFontSize int
)

var watermarkCmd = &cobra.Command{
	Use:   "watermark <files...> <text>",
	Short: "Add text watermark to images",
	Long: `Add a text watermark to one or more images.

Examples:
  fc watermark photo.jpg "Copyright 2024"
  fc watermark *.jpg "DRAFT" --position center --opacity 30
  fc watermark photo.jpg "Sample" --font-size 48`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := args[len(args)-1]
		files := args[:len(args)-1]

		var expanded []string
		for _, f := range files {
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
				Processor: "watermark",
				Options: &processor.Options{
					VariantType: text,
					Fit:         wmPosition,
					Quality:     wmOpacity,
					Width:       wmFontSize,
				},
			}
			if cfg.OutputDir != "" {
				req.OutputPath = filepath.Join(cfg.OutputDir, filepath.Base(file))
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
	watermarkCmd.Flags().StringVar(&wmPosition, "position", "bottom-right", "Position: top-left, top-right, bottom-left, bottom-right, center")
	watermarkCmd.Flags().IntVar(&wmOpacity, "opacity", 50, "Opacity (1-100)")
	watermarkCmd.Flags().IntVar(&wmFontSize, "font-size", 24, "Font size in pixels")
}
