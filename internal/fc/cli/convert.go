package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var convertCmd = &cobra.Command{
	Use:   "convert <files...> <format>",
	Short: "Convert files to a different format",
	Long: `Convert images to a different format (jpeg, png, gif, webp).

Examples:
  fcheap convert photo.jpg webp
  fcheap convert photo.png jpeg --quality 90
  fcheap convert *.png webp`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		format := strings.ToLower(args[len(args)-1])
		files := args[:len(args)-1]

		// Expand globs
		var expanded []string
		for _, f := range files {
			matches, err := filepath.Glob(f)
			if err != nil || len(matches) == 0 {
				expanded = append(expanded, f)
			} else {
				expanded = append(expanded, matches...)
			}
		}

		procName := "convert"
		if format == "webp" {
			procName = "webp"
		}

		reqs := make([]*engine.Request, len(expanded))
		for i, file := range expanded {
			opts := &processor.Options{
				Format:  format,
				Quality: cfg.Quality,
			}
			req := &engine.Request{
				InputPath: file,
				Processor: procName,
				Options:   opts,
			}
			if cfg.OutputDir != "" {
				base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
				ext := format
				if ext == "jpeg" {
					ext = "jpg"
				}
				req.OutputPath = filepath.Join(cfg.OutputDir, base+"."+ext)
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

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
