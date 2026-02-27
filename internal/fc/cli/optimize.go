package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var optimizeCmd = &cobra.Command{
	Use:   "optimize <files...>",
	Short: "Optimize images for smaller file size",
	Long: `Optimize images by re-encoding with quality settings.
Reduces file size while maintaining visual quality.

Examples:
  fcheap optimize photo.jpg
  fcheap optimize *.jpg --quality 80
  fcheap optimize photos/ -q 75`,
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
				Processor: "optimize",
				Options: &processor.Options{
					Quality: cfg.Quality,
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
		var totalSaved int64
		for i, res := range results {
			if errs[i] != nil {
				printer.FileFailed(expanded[i], errs[i])
				failed++
				continue
			}
			successful++
			saved := res.InputSize - res.OutputSize
			totalSaved += saved
			pct := float64(0)
			if res.InputSize > 0 {
				pct = float64(saved) / float64(res.InputSize) * 100
			}
			printer.Success("%s -> %s (saved %s, %.0f%%)", expanded[i], res.OutputPath, formatSize(saved), pct)
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{
				"successful": successful, "failed": failed,
				"total_saved": totalSaved, "results": results,
			})
		}
		if len(expanded) > 1 {
			printer.Summary(successful, failed)
			if totalSaved > 0 {
				printer.Info("Total saved: %s", formatSize(totalSaved))
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d file(s) failed", failed)
		}
		return nil
	},
}
