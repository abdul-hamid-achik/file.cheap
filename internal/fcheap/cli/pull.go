package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/cloudartifact"
	"github.com/abdul-hamid-achik/file.cheap/internal/cloudauth"
	"github.com/spf13/cobra"
)

var pullOutputPath string

var pullCmd = &cobra.Command{
	Use:   "pull <artifact-id>",
	Short: "Download and verify one private cloud artifact",
	Long: `Download one committed artifact with the paired device credential.

The command requests a short-lived direct transfer, streams no more than the
recorded size, verifies SHA-256, and installs the bytes at an explicit new path.
It never prints or persists the signed transfer URL and never overwrites an
existing file. It does not extract or preview untrusted archive contents.`,
	Example: `  fcheap auth login
  fcheap pull art_abcdefghijklmnop --output ./run.tar.zst
  fcheap pull art_abcdefghijklmnop --output ./run.tar.zst --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		credentials, err := cloudauth.Load()
		if err != nil {
			return err
		}
		_, credentials, err = sessionWithRefresh(
			cloudauth.NewClient(nil),
			credentials,
		)
		if err != nil {
			return err
		}
		outputPath, err := filepath.Abs(pullOutputPath)
		if err != nil {
			return fmt.Errorf("resolve pull output: %w", err)
		}
		result, err := cloudartifact.NewClient(nil).Pull(
			GetContext(),
			cloudartifact.Options{
				ArtifactID:  args[0],
				Destination: outputPath,
				ServiceURL:  credentials.ServiceURL,
				Token:       credentials.Token,
			},
		)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			return printer.JSON(result)
		}
		printer.Success("Downloaded and verified private artifact")
		printer.KeyValue("Artifact", result.ArtifactRef.ArtifactID)
		printer.KeyValue("Output", result.OutputPath)
		printer.KeyValue("SHA-256", result.SHA256)
		printer.KeyValue("Size", formatSize(result.SizeBytes))
		printer.KeyValue("Verification", result.Verification)
		return nil
	},
}

func init() {
	pullCmd.Flags().StringVarP(
		&pullOutputPath,
		"output",
		"o",
		"",
		"New file path for the verified artifact bytes (required)",
	)
	_ = pullCmd.MarkFlagRequired("output")
}
