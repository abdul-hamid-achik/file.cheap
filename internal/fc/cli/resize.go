package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/presets"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var resizeFit string

var resizeCmd = &cobra.Command{
	Use:   "resize <files...> <size>",
	Short: "Resize images",
	Long: `Resize images to a specified size.

Size can be:
  WxH     - exact dimensions (e.g., 800x600)
  W       - width only, maintain aspect ratio (e.g., 800)
  preset  - sm (640), md (1024), lg (1920), xl (2560)

Examples:
  fcheap resize photo.jpg 800x600
  fcheap resize photo.jpg 800
  fcheap resize *.jpg md
  fcheap resize photo.jpg 1024 --fit cover`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sizeSpec := args[len(args)-1]
		files := args[:len(args)-1]

		width, height, err := parseSize(sizeSpec)
		if err != nil {
			return err
		}

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
				Processor: "resize",
				Options: &processor.Options{
					Width:   width,
					Height:  height,
					Quality: cfg.Quality,
					Fit:     resizeFit,
				},
			}
			if cfg.OutputDir != "" {
				base := filepath.Base(file)
				req.OutputPath = filepath.Join(cfg.OutputDir, base)
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
			printer.Success("%s -> %s (%dx%d, %s)", expanded[i], res.OutputPath, res.Width, res.Height, formatSize(res.OutputSize))
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
	resizeCmd.Flags().StringVar(&resizeFit, "fit", "", "Fit mode: contain (default), cover, fill")
}

func parseSize(spec string) (int, int, error) {
	// Check preset names
	if p, ok := presets.Get(spec); ok {
		return p.Width, p.Height, nil
	}

	// Named sizes
	switch spec {
	case "sm":
		return 640, 0, nil
	case "md":
		return 1024, 0, nil
	case "lg":
		return 1920, 0, nil
	case "xl":
		return 2560, 0, nil
	}

	// WxH format
	if strings.Contains(spec, "x") {
		parts := strings.SplitN(spec, "x", 2)
		w, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid width: %s", parts[0])
		}
		h, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid height: %s", parts[1])
		}
		return w, h, nil
	}

	// Width only
	w, err := strconv.Atoi(spec)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid size: %s (use WxH, width, or preset name)", spec)
	}
	return w, 0, nil
}
