package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var ttlCmd = &cobra.Command{
	Use:   "ttl <stash-id> <duration>",
	Short: "Set or update the time-to-live for a stash",
	Long: `Set the TTL on an existing stash. The stash will be marked expired after
the given duration from its creation time; no automatic deletion occurs. Use
"fcheap sweep --apply" to actually drop expired stashes.

Duration examples: 7d (7 days), 24h (24 hours), 30d, 2w (2 weeks), 2026-12-31.
Pass an empty string "" to clear the TTL (make the stash permanent).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}

		id := args[0]
		if !mgr.Exists(id) {
			return fmt.Errorf("stash not found: %s", id)
		}

		ttl := ""
		if len(args) > 1 {
			ttl = args[1]
		}

		if err := mgr.SetExpiry(GetContext(), id, ttl); err != nil {
			return err
		}

		// Read back the updated manifest for display.
		st, _ := mgr.Info(GetContext(), id)

		if printer.IsJSON() {
			return printer.JSON(map[string]string{
				"stash_id":   id,
				"expires_at": st.Manifest.ExpiresAt,
			})
		}

		if st.Manifest.ExpiresAt == "" {
			printer.Success("Cleared TTL on stash: %s (permanent)", id)
		} else {
			printer.Success("Set TTL on stash: %s", id)
			printer.KeyValue("Expires", st.Manifest.ExpiresAt)
		}
		return nil
	},
}
