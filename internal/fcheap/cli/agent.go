package cli

import (
	"github.com/abdul-hamid-achik/file.cheap/internal/agentguide"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Print the operating guide for AI agents",
	Long: `Print a concise, local-first operating guide for AI agents using fcheap.

The default output is human-readable. Use --json for the stable, versioned
machine-readable contract shared with the MCP server. This command reads no
vault data and does not require a valid fcheap configuration.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		guide := agentguide.New(version.Short())
		if printer.IsJSON() {
			return printer.JSON(guide)
		}
		printer.Printf("%s", agentguide.Render(guide))
		return nil
	},
}
