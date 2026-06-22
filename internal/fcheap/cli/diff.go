package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <stash-id> <target-dir>",
	Short: "Compare a stash against a target directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		if !mgr.Exists(args[0]) {
			return fmt.Errorf("stash not found: %s", args[0])
		}

		stashDir := mgr.StashDir(args[0])
		targetAbs, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		if _, err := os.Stat(targetAbs); err != nil {
			return fmt.Errorf("target not found: %w", err)
		}

		result, err := diff.CompareStashToDir(stashDir, targetAbs)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(result)
		}

		printer.Header("Diff: " + args[0] + " vs " + targetAbs)
		printer.Println(result.Format())
		return nil
	},
}