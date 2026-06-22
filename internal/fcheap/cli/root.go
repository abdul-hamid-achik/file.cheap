package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	quietMode  bool
	stashDir   string

	cfg     *config.Config
	printer *output.Printer

	rootCtx    context.Context
	rootCancel context.CancelFunc
)

var rootCmd = &cobra.Command{
	Use:   "fcheap",
	Short: "file.cheap - local-first stash tool for saving, restoring, and analyzing files",
	Long: `fcheap saves, restores, compresses, and analyzes files and folders for agent workflows.

Get started:
  fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace
  fcheap list --tag bug-123
  fcheap info <stash-id>
  fcheap restore <stash-id> --to /tmp/working/
  fcheap drop <stash-id>
  fcheap search "error message"
  fcheap diff <stash-id> ~/projects/graphite
  fcheap analyze <stash-id> --query "keyword"
  fcheap studio
  fcheap mcp serve
  fcheap docs serve
  fcheap doctor`,
	Version: version.Full(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		rootCtx, rootCancel = context.WithCancel(context.Background())

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			if printer != nil && !quietMode {
				printer.Warn("\nReceived %s, cancelling...", sig)
			}
			rootCancel()
		}()

		if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "completion" {
			return nil
		}

		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}

		if stashDir != "" {
			cfg.StashDir = stashDir
		}

		printer = output.New(
			output.WithJSON(jsonOutput),
			output.WithQuiet(quietMode),
		)

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if rootCancel != nil {
			rootCancel()
		}
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress non-error output")
	rootCmd.PersistentFlags().StringVar(&stashDir, "stash-dir", "", "Stash storage directory (overrides config)")

	rootCmd.SetVersionTemplate("fcheap version {{.Version}}\n")

	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(dropCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(compressCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(studioCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(completionCmd)
}

// GetContext returns the root context for the CLI command.
func GetContext() context.Context {
	if rootCtx == nil {
		return context.Background()
	}
	return rootCtx
}