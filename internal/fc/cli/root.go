package cli

import (
	"os"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/client"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/version"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	quietMode  bool
	cfg        *config.Config
	apiClient  *client.Client
	printer    *output.Printer
)

var rootCmd = &cobra.Command{
	Use:   "fc",
	Short: "file.cheap CLI - upload, transform, and deliver images",
	Long: `fc is the command-line interface for file.cheap.

Upload files, apply transformations, and manage your images from the terminal.

Get started:
  fc auth login              # Authenticate with file.cheap
  fc upload photo.jpg        # Upload a file
  fc list                    # List your files`,
	Version: version.Full(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" || cmd.Name() == "version" {
			return nil
		}

		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}

		printer = output.New(
			output.WithJSON(jsonOutput),
			output.WithQuiet(quietMode),
		)

		apiClient = client.New(cfg.BaseURL, cfg.APIKey)
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON (for scripting)")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress non-error output")

	rootCmd.SetVersionTemplate("fc version {{.Version}}\n")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(transformCmd)
	rootCmd.AddCommand(batchCmd)
	rootCmd.AddCommand(socialCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(shareCmd)
}

func requireAuth() error {
	if !cfg.IsAuthenticated() {
		printer.Error("Not authenticated. Run 'fc auth login' first.")
		os.Exit(1)
	}
	return nil
}
