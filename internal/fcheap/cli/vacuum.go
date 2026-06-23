package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var vacuumCmd = &cobra.Command{
	Use:   "vacuum",
	Short: "Remove orphaned index entries and compact the database",
	Long: `vacuum reclaims storage by removing metadata- and search-index entries for
stashes whose directory no longer exists (e.g. deleted outside 'fcheap drop'),
then compacts the SQLite database.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}
		an := analyze.NewAnalyzer(cfg.StashDir, cfg.VecgrepPath)

		res, err := mgr.Vacuum(GetContext(), an.DropIndex)
		if err != nil {
			return err
		}

		if printer.IsJSON() {
			return printer.JSON(res)
		}

		printer.Success("Vacuum complete")
		printer.KeyValue("On disk", fmt.Sprintf("%d stash(es)", res.OnDisk))
		printer.KeyValue("Orphans removed", fmt.Sprintf("%d", res.OrphanedRows))
		for _, id := range res.Orphans {
			printer.Indent("%s", id)
		}
		return nil
	},
}
