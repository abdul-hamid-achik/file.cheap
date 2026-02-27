package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <files...>",
	Short: "Show file metadata",
	Long: `Display metadata for images (dimensions, format, size).

Examples:
  fcheap info photo.jpg
  fcheap info *.jpg
  fcheap info photo.jpg --json`,
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

		type fileInfo struct {
			Path        string `json:"path"`
			Size        int64  `json:"size"`
			ContentType string `json:"content_type"`
			Width       int    `json:"width,omitempty"`
			Height      int    `json:"height,omitempty"`
			Format      string `json:"format,omitempty"`
		}

		var infos []fileInfo
		for _, file := range expanded {
			stat, err := os.Stat(file)
			if err != nil {
				printer.FileFailed(file, err)
				continue
			}

			ct, _ := engine.DetectContentType(file)

			info := fileInfo{
				Path:        file,
				Size:        stat.Size(),
				ContentType: ct,
			}

			// Try to get image metadata
			res, err := eng.Process(GetContext(), &engine.Request{
				InputPath: file,
				Processor: "metadata",
				Options:   &processor.Options{},
			})
			if err == nil && res != nil {
				info.Width = res.Width
				info.Height = res.Height
				info.Format = res.Format

				// Also try to read the metadata JSON from result
				if res.Metadata.Width > 0 {
					info.Width = res.Metadata.Width
					info.Height = res.Metadata.Height
					info.Format = res.Metadata.Format
				}
			}

			infos = append(infos, info)

			if !jsonOutput {
				printer.Section(filepath.Base(file))
				printer.KeyValue("Path", file)
				printer.KeyValue("Size", formatSize(stat.Size()))
				printer.KeyValue("Type", ct)
				if info.Width > 0 {
					printer.KeyValue("Dimensions", fmt.Sprintf("%dx%d", info.Width, info.Height))
				}
				if info.Format != "" {
					printer.KeyValue("Format", info.Format)
				}
			}
		}

		if jsonOutput {
			return printer.PrintResult(infos)
		}
		return nil
	},
}
