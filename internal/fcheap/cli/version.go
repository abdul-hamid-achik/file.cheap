package cli

import (
	"encoding/json"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the fcheap version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// The root PersistentPreRunE skips printer setup for `version`, so read
		// the --json flag directly rather than relying on the (nil) printer.
		if jsonOutput {
			data, err := json.MarshalIndent(map[string]string{
				"version": version.Version,
				"commit":  version.Commit,
				"date":    version.Date,
			}, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}
		cmd.Printf("fcheap %s\n", version.Full())
		return nil
	},
}
