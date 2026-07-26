package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	quietMode  bool
	noColor    bool
	stashDir   string
	logLevel   string

	cfg     *config.Config
	printer *output.Printer

	rootCtx    context.Context
	rootCancel context.CancelFunc
)

var rootCmd = &cobra.Command{
	Use:   "fcheap",
	Short: "file.cheap - the local artifact vault for coding agents",
	Long: `fcheap saves, restores, compresses, and analyzes files and folders for agent workflows.

Core workflow:
  fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace --index
  fcheap list --tag bug-123
  fcheap search "error message"
  fcheap info <stash-id>
  fcheap restore <stash-id>
  fcheap connect <stash-id> ~/projects/my-app

Delete only when intended:
  fcheap drop <stash-id> --force

For agents and integrations:
  fcheap agent
  fcheap agent --json
  fcheap mcp serve
  fcheap docs show mcp/overview

Pair with the private artifact console:
  fcheap auth login
  fcheap auth status

Explore interactively:
  fcheap studio
  fcheap doctor`,
	Version: version.Full(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		rootCtx, rootCancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		// Restore the process's default signal behavior as soon as the first
		// interrupt cancels the operation. A second Ctrl+C can then terminate a
		// command whose third-party work has not yet observed the context.
		go func(ctx context.Context, stop context.CancelFunc) {
			<-ctx.Done()
			stop()
		}(rootCtx, rootCancel)

		printer = output.New(
			output.WithJSON(jsonOutput),
			output.WithQuiet(quietMode),
			output.WithNoColor(noColor || os.Getenv("NO_COLOR") != ""),
		)

		if !commandNeedsConfig(cmd) {
			lvl := config.DefaultLogLevel
			if logLevel != "" {
				lvl = logLevel
			}
			logger.Init(lvl)
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

		// Configure logging (stderr); --log-level overrides the config value.
		lvl := cfg.LogLevel
		if logLevel != "" {
			lvl = logLevel
		}
		logger.Init(lvl)

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
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output (also honors NO_COLOR)")
	rootCmd.PersistentFlags().StringVar(&stashDir, "stash-dir", "", "Stash storage directory (overrides config)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error (to stderr)")

	rootCmd.SetVersionTemplate("fcheap version {{.Version}}\n")

	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(dropCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(artifactRefCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(compressCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(vacuumCmd)
	rootCmd.AddCommand(ttlCmd)
	rootCmd.AddCommand(sweepCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(ecosystemStatusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(studioCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(versionCmd)
}

// commandNeedsConfig reports whether a command operates on the configured
// vault. Static help, agent guidance, and embedded documentation remain usable
// even when the user's vault configuration is missing or invalid.
func commandNeedsConfig(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	switch cmd.Name() {
	case "help", "version", "completion", "agent", "auth", "login", "status", "refresh", "logout", "publish":
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		if current == docsCmd {
			return false
		}
	}
	return true
}

// GetContext returns the root context for the CLI command.
func GetContext() context.Context {
	if rootCtx == nil {
		return context.Background()
	}
	return rootCtx
}

// embSettings builds the analyzer embedder settings from the loaded config.
func embSettings() analyze.EmbedderSettings {
	if cfg == nil {
		return analyze.EmbedderSettings{}
	}
	return analyze.EmbedderSettings{
		Provider:           cfg.Embedder,
		Model:              cfg.EmbedModel,
		URL:                cfg.OllamaURL,
		AllowSecretContent: cfg.AllowRemoteSecrets,
	}
}
