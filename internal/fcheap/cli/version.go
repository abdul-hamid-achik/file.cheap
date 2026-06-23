package cli

import (
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the fcheap version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if printer != nil && printer.IsJSON() {
			return printer.JSON(map[string]string{
				"version": version.Version,
				"commit":  version.Commit,
				"date":    version.Date,
			})
		}
		cmd.Printf("fcheap %s\n", version.Full())
		return nil
	},
}
