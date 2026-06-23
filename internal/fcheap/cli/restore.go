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

		res, err := mgr.Restore(GetContext(), args[0], restoreTarget)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]any{
				"stash_id":   args[0],
				"target":     res.Target,
				"file_count": res.FileCount,
				"verified":   res.Verified,
				"mismatches": res.Mismatches,
				"status":     "restored",
			})
		}

		printer.Success("Restored %s to %s", args[0], res.Target)
		printer.KeyValue("Files", fmt.Sprintf("%d", res.FileCount))
		if res.Verified {
			printer.KeyValue("Verified", "all files match manifest hashes")
		} else {
			printer.Warn("%d file(s) failed verification:", len(res.Mismatches))
			for _, mm := range res.Mismatches {
				printer.Indent("%s", mm)
			}
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().StringVar(&restoreTarget, "to", "", "Target directory (default: /tmp/<stash-id>)")
}
