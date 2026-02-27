package cli

import (
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system dependencies",
	Long: `Check that required external tools are installed and available.

fcheap uses these optional dependencies:
  ffmpeg/ffprobe  - Video processing (transcode, thumbnail, HLS, watermark)
  pdftoppm        - PDF thumbnail generation (from poppler-utils)
  pdfinfo         - PDF page counting (from poppler-utils)
  cwebp           - WebP conversion (from libwebp-tools)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		type depInfo struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			Path      string `json:"path,omitempty"`
			Version   string `json:"version,omitempty"`
			Purpose   string `json:"purpose"`
		}

		deps := []struct {
			name       string
			purpose    string
			versionCmd []string
		}{
			{"ffmpeg", "Video processing", []string{"ffmpeg", "-version"}},
			{"ffprobe", "Video metadata", []string{"ffprobe", "-version"}},
			{"pdftoppm", "PDF thumbnails", []string{"pdftoppm", "-v"}},
			{"pdfinfo", "PDF page counting", []string{"pdfinfo", "-v"}},
			{"cwebp", "WebP conversion", []string{"cwebp", "-version"}},
		}

		printer.Header("System Dependencies")

		var results []depInfo
		allGood := true

		for _, dep := range deps {
			info := depInfo{
				Name:    dep.name,
				Purpose: dep.purpose,
			}

			path, err := exec.LookPath(dep.name)
			if err != nil {
				info.Available = false
				allGood = false
				printer.Error("%-10s not found  (%s)", dep.name, dep.purpose)
			} else {
				info.Available = true
				info.Path = path

				// Try to get version
				out, err := exec.Command(dep.versionCmd[0], dep.versionCmd[1:]...).CombinedOutput()
				if err == nil {
					lines := strings.SplitN(string(out), "\n", 2)
					if len(lines) > 0 {
						info.Version = strings.TrimSpace(lines[0])
					}
				}

				printer.Success("%-10s %s", dep.name, info.Path)
				if info.Version != "" {
					printer.Indent("%s", info.Version)
				}
			}

			results = append(results, info)
		}

		// Also show registered processors
		printer.Section("Registered Processors")
		for _, p := range eng.ListProcessors() {
			printer.Info("%-20s %s", p.Name, strings.Join(p.SupportedTypes, ", "))
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{
				"dependencies": results,
				"processors":   eng.ListProcessors(),
				"all_ok":       allGood,
			})
		}

		if allGood {
			printer.Println()
			printer.Success("All dependencies found")
		} else {
			printer.Println()
			printer.Warn("Some dependencies are missing. Install them for full functionality.")
		}

		return nil
	},
}
