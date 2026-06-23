package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
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
			if printer.IsJSON() {
				_ = printer.JSON(map[string]string{
					"stash_id": args[0],
					"status":   "not_confirmed",
					"requires": "--force",
				})
			} else {
				printer.Warn("This will permanently delete stash: %s", args[0])
				printer.Warn("Use --force to confirm.")
			}
			// Return a non-zero error so --json/--quiet callers can detect that
			// nothing was deleted (a silent exit 0 looked like success).
			return fmt.Errorf("refusing to drop stash %s without --force", args[0])
		}

		if err := mgr.Drop(GetContext(), args[0]); err != nil {
			return err
		}
		// Best-effort: remove any indexed documents for this stash.
		_ = analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath).DropIndex(args[0])

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
