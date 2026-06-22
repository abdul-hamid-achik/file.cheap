# mcp

Start the MCP (Model Context Protocol) server for AI assistant integration.

## Usage

```bash
fcheap mcp serve
```

## Transport

The MCP server uses stdio transport, which is the standard for local MCP servers. AI assistants like Claude communicate with it over stdin/stdout.

## Claude Code Configuration

Add fcheap to your Claude Code MCP config:

```json
{
  "mcpServers": {
    "file-cheap": {
      "command": "fcheap",
      "args": ["mcp", "serve"]
    }
  }
}
```

## Available Tools

| Tool | Description |
|------|-------------|
| `fcheap_save` | Save a file or directory to the stash vault |
| `fcheap_list` | List all stashes, optionally filtered by tag |
| `fcheap_info` | Get detailed info about a stash |
| `fcheap_restore` | Restore a stash to a target directory |
| `fcheap_drop` | Permanently delete a stash (requires force=true) |
| `fcheap_search` | Search across all indexed stashes |
| `fcheap_analyze` | Index a stash and optionally search within it |
| `fcheap_diff` | Compare a stash against a target directory |

## Tool Schemas

All tools use typed input structs with JSON schema auto-generation. The `jsonschema` tag provides the description, and `json` tag `omitempty` controls required vs optional.

```go
type saveInput struct {
    Path string `json:"path" jsonschema:"Absolute path to the file or directory"`
    Name string `json:"name,omitempty" jsonschema:"Display name for the stash"`
}
```