package cli

import (
	"github.com/spf13/cobra"

	fcmcp "github.com/abdul-hamid-achik/file.cheap/internal/mcp"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/version"
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

This allows AI assistants like Claude to use fc as a tool for
processing images, PDFs, and videos.

Usage in Claude Code MCP config:
  {
    "mcpServers": {
      "file-cheap": {
        "command": "fc",
        "args": ["mcp", "serve"]
      }
    }
  }`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := fcmcp.NewServer(eng, version.Short())
		return s.Run(GetContext(), &mcp.StdioTransport{})
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}
