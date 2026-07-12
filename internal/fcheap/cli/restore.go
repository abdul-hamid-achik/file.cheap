package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	restoreTarget        string
	restoreAllowMismatch bool
)

type restoreOutput struct {
	StashID    string   `json:"stash_id"`
	Target     string   `json:"target"`
	FileCount  int      `json:"file_count"`
	Verified   bool     `json:"verified"`
	Mismatches []string `json:"mismatches"`
	Status     string   `json:"status"`
}

var restoreCmd = &cobra.Command{
	Use:   "restore <stash-id>",
	Short: "Restore a stash to a directory",
	Long: `Restore a stash and verify every restored entry against its manifest hash.

Without --to, fcheap creates a fresh private temporary directory. When --to
names an existing directory, matching files are replaced; unrelated files are
left in place. Targets that overlap the vault in either direction are rejected.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		res, err := mgr.Restore(GetContext(), args[0], restoreTarget)
		if err != nil {
			return err
		}

		out := restoreOutput{
			StashID:    args[0],
			Target:     res.Target,
			FileCount:  res.FileCount,
			Verified:   res.Verified,
			Mismatches: res.Mismatches,
			Status:     "restored",
		}
		if out.Mismatches == nil {
			out.Mismatches = []string{}
		}
		if !out.Verified {
			out.Status = "restored_unverified"
			if len(out.Mismatches) > 0 {
				out.Status = "restored_with_mismatches"
			}
		}

		if printer.IsJSON() {
			if err := printer.JSON(out); err != nil {
				return err
			}
		} else {
			if res.Verified {
				printer.Success("Restored %s to %s", args[0], res.Target)
			} else {
				printer.Warn("Restored %s to %s, but integrity verification failed", args[0], res.Target)
			}
			printer.KeyValue("Files", fmt.Sprintf("%d", res.FileCount))
			if res.Verified {
				printer.KeyValue("Verified", "all files match manifest hashes")
			} else {
				printer.Warn("%d file(s) failed verification:", len(res.Mismatches))
				for _, mm := range res.Mismatches {
					printer.Indent("%s", mm)
				}
			}
		}

		// Restoring bytes and verifying them are one command contract. A mismatch is
		// therefore a non-zero result by default, after the caller has received the
		// complete structured/human-readable restore outcome. The explicit escape
		// hatch is useful for forensic recovery of known-corrupt snapshots.
		if !res.Verified && !restoreAllowMismatch {
			return fmt.Errorf("restore verification failed: %d mismatch(es); use --allow-mismatch to accept the restored files", len(res.Mismatches))
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().StringVar(&restoreTarget, "to", "", "Target directory (default: a fresh, unique temp directory)")
	restoreCmd.Flags().BoolVar(&restoreAllowMismatch, "allow-mismatch", false, "Exit successfully even if restored files fail manifest verification")
}
