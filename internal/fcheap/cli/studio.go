package cli

import (
	"github.com/abdul-hamid-achik/file.cheap/internal/studio"
	"github.com/spf13/cobra"
)

var studioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Open the Studio TUI for browsing stashes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return studio.Run(cfg.StashDir, cfg.VecgrepPath)
	},
}