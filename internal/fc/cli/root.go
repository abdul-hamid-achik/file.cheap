package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/version"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	quietMode  bool
	outputDir  string
	quality    int
	parallel   int
	overwrite  bool

	cfg     *config.Config
	printer *output.Printer
	eng     *engine.Engine

	rootCtx    context.Context
	rootCancel context.CancelFunc
)

var rootCmd = &cobra.Command{
	Use:   "fc",
	Short: "file.cheap - local file processing CLI and MCP server",
	Long: `fc processes images, PDFs, and videos locally on your machine.

Get started:
  fc convert photo.jpg webp       # Convert to WebP
  fc resize photo.jpg 800x600     # Resize an image
  fc thumbnail photo.jpg          # Generate thumbnail
  fc optimize *.jpg               # Optimize images
  fc info photo.jpg               # Show file metadata
  fc doctor                       # Check dependencies
  fc mcp serve                    # Start MCP server`,
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

		// CLI flags override config
		if quality > 0 {
			cfg.Quality = quality
		}
		if parallel > 0 {
			cfg.Parallel = parallel
		}
		if outputDir != "" {
			cfg.OutputDir = outputDir
		}
		if overwrite {
			cfg.Overwrite = true
		}

		printer = output.New(
			output.WithJSON(jsonOutput),
			output.WithQuiet(quietMode),
		)

		procCfg := &processor.Config{
			MaxFileSize:  100 * 1024 * 1024,
			TempDir:      cfg.TempDir,
			Quality:      cfg.Quality,
			MaxDimension: 4096,
		}
		if procCfg.TempDir == "" {
			procCfg.TempDir = os.TempDir()
		}

		eng = engine.New(procCfg)
		eng.RegisterDefaults()

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
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "", "Output directory")
	rootCmd.PersistentFlags().IntVarP(&quality, "quality", "q", 0, "Quality (1-100)")
	rootCmd.PersistentFlags().IntVarP(&parallel, "parallel", "j", 0, "Parallel workers")
	rootCmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing files")

	rootCmd.SetVersionTemplate("fc version {{.Version}}\n")

	rootCmd.AddCommand(convertCmd)
	rootCmd.AddCommand(resizeCmd)
	rootCmd.AddCommand(thumbnailCmd)
	rootCmd.AddCommand(optimizeCmd)
	rootCmd.AddCommand(watermarkCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(processCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(completionCmd)
}

// GetContext returns the root context for the CLI command.
func GetContext() context.Context {
	if rootCtx == nil {
		return context.Background()
	}
	return rootCtx
}
