package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var processTransforms string

var processCmd = &cobra.Command{
	Use:   "process <files...>",
	Short: "Apply one or more transformations",
	Long: `Apply a chain of transformations to files.

Examples:
  fc process photo.jpg -t resize,webp
  fc process *.jpg -t optimize,thumbnail
  fc process photo.jpg -t resize --quality 80`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if processTransforms == "" {
			return fmt.Errorf("specify transforms with -t (e.g., -t resize,webp)")
		}

		transforms := strings.Split(processTransforms, ",")
		for i := range transforms {
			transforms[i] = strings.TrimSpace(transforms[i])
		}

		var expanded []string
		for _, f := range args {
			matches, err := filepath.Glob(f)
			if err != nil || len(matches) == 0 {
				expanded = append(expanded, f)
			} else {
				expanded = append(expanded, matches...)
			}
		}

		successful := 0
		failed := 0

		for _, file := range expanded {
			currentInput := file
			var lastResult *engine.Result

			for _, transform := range transforms {
				res, err := eng.Process(GetContext(), &engine.Request{
					InputPath: currentInput,
					Processor: transform,
					Options: &processor.Options{
						Quality: cfg.Quality,
					},
				})
				if err != nil {
					printer.FileFailed(file, fmt.Errorf("%s: %w", transform, err))
					failed++
					lastResult = nil
					break
				}
				lastResult = res
				if res.OutputPath != "" {
					currentInput = res.OutputPath
				}
			}

			if lastResult != nil {
				successful++
				printer.Success("%s -> %s (%s)", file, lastResult.OutputPath, formatSize(lastResult.OutputSize))
			}
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{"successful": successful, "failed": failed})
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
	processCmd.Flags().StringVarP(&processTransforms, "transforms", "t", "", "Comma-separated list of transforms")
}
