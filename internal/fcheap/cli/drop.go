package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var dropForce bool

var dropCmd = &cobra.Command{
	Use:   "drop <stash-id>",
	Short: "Drop (delete) a stash permanently",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if !mgr.Exists(args[0]) {
			return fmt.Errorf("stash not found: %s", args[0])
		}

		if !dropForce {
			printer.Warn("This will permanently delete stash: %s", args[0])
			printer.Warn("Use --force to confirm.")
			return nil
		}

		if err := mgr.Drop(GetContext(), args[0]); err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]string{
				"stash_id": args[0],
				"status":   "dropped",
			})
		}

		printer.Success("Dropped stash: %s", args[0])
		return nil
	},
}

func init() {
	dropCmd.Flags().BoolVar(&dropForce, "force", false, "Force deletion without confirmation")
}