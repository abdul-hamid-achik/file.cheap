package cli

import (
	"github.com/spf13/cobra"

	fcmcp "github.com/abdul-hamid-achik/file.cheap/internal/mcp"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server commands",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server (stdio transport)",
	Long: `Start an MCP (Model Context Protocol) server using stdio transport.

This allows AI assistants like Claude to use fcheap as a tool for
saving, restoring, and analyzing files.

Usage in Claude Code MCP config:
  {
    "mcpServers": {
      "file-cheap": {
        "command": "fcheap",
        "args": ["mcp", "serve"]
      }
    }
  }`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := fcmcp.NewServer(cfg.StashDir, cfg.VecgrepPath, version.Short())
		return s.Run(GetContext(), &mcp.StdioTransport{})
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}