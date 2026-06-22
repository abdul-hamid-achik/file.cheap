package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var restoreTarget string

var restoreCmd = &cobra.Command{
	Use:   "restore <stash-id>",
	Short: "Restore a stash to a directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if err := mgr.Restore(GetContext(), args[0], restoreTarget); err != nil {
			return err
		}

		target := restoreTarget
		if target == "" {
			target = fmt.Sprintf("/tmp/%s", args[0])
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]string{
				"stash_id": args[0],
				"target":   target,
				"status":   "restored",
			})
		}

		printer.Success("Restored %s to %s", args[0], target)
		return nil
	},
}

func init() {
	restoreCmd.Flags().StringVar(&restoreTarget, "to", "", "Target directory (default: /tmp/<stash-id>)")
}