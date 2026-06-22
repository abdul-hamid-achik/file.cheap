package cli

import (
	"os/exec"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system dependencies and configuration",
	Args:  cobra.NoArgs,
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
			{"vecgrep", "Semantic search (optional)", []string{"vecgrep", "--version"}},
			{"zstd", "Standalone zstd compression", []string{"zstd", "--version"}},
			{"tar", "Archive creation", []string{"tar", "--version"}},
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
				printer.Warn("%-10s not found  (%s)", dep.name, dep.purpose)
			} else {
				info.Available = true
				info.Path = path

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

		// Check stash directory
		printer.Section("Stash Directory")
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			printer.Error("Cannot create stash directory: %v", err)
			allGood = false
		} else {
			printer.Success("Stash dir: %s", mgr.RootDir())
			stashes, err := mgr.List(GetContext(), "")
			if err != nil {
				printer.Error("Cannot list stashes: %v", err)
			} else {
				printer.Indent("%d stash(es)", len(stashes))
			}
		}

		if jsonOutput {
			return printer.PrintResult(map[string]any{
				"dependencies": results,
				"stash_dir":    cfg.StashDir,
				"all_ok":       allGood,
			})
		}

		printer.Println()
		printer.Success("fcheap doctor: ok")
		return nil
	},
}