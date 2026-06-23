package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
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

		// Check stash directory and storage indexes
		printer.Section("Stash Storage")
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			printer.Error("Cannot create stash directory: %v", err)
			allGood = false
		} else {
			printer.Success("Stash dir: %s", mgr.RootDir())
			count, total := mgr.Stats(GetContext())
			printer.Indent("%d stash(es), %s total", count, formatSize(total))

			dbPath := filepath.Join(mgr.RootDir(), "fcheap.db")
			if _, err := os.Stat(dbPath); err == nil {
				printer.Success("metadata index (SQLite): %s", dbPath)
			} else {
				printer.Indent("metadata index not yet created (run a save)")
			}

			vecPath := filepath.Join(mgr.RootDir(), "fcheap.veclite")
			if _, err := os.Stat(vecPath); err == nil {
				printer.Success("search index (veclite): %s", vecPath)
			} else {
				printer.Indent("search index not yet created (run analyze)")
			}

			// Embedder (semantic/hybrid search).
			if cfg.Embedder != "" {
				an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).WithEmbedder(embSettings())
				if dim, err := an.CheckEmbedder(); err != nil {
					printer.Warn("embedder (%s/%s): unreachable — %v", cfg.Embedder, cfg.EmbedModel, err)
				} else {
					printer.Success("embedder (%s/%s): available, %d-dim", cfg.Embedder, cfg.EmbedModel, dim)
				}
			} else {
				printer.Indent("embedder: not configured (keyword search only)")
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
