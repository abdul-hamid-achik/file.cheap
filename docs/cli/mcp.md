# mcp

Start the MCP (Model Context Protocol) server for AI assistant integration.

Run `fcheap agent` before setup to print the same concise operating guide the
server provides to clients during initialization.

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
| `fcheap_list` | List active stashes, optionally filtered by tag; expired stashes are opt-in |
| `fcheap_info` | Get detailed info about a stash |
| `fcheap_restore` | Restore a stash to a target directory |
| `fcheap_drop` | Permanently delete a stash (requires force=true) |
| `fcheap_search` | Search across all indexed stashes (keyword / semantic / hybrid) |
| `fcheap_analyze` | Index a stash and optionally search within it |
| `fcheap_diff` | Compare a stash against a target directory |
| `fcheap_connect` | Connect a stash to a codebase via vecgrep semantic search to surface candidate `file:line` locations |
| `fcheap_vacuum` | Remove orphaned metadata/index entries for stashes whose directory is gone, then compact the DB |
| `fcheap_ttl` | Set or clear a stash expiry |
| `fcheap_sweep` | Plan or apply deletion of expired stashes |
| `fcheap_cleanup` | Analyze cleanup candidates; apply only explicit TTLs or regenerable caches |
| `fcheap_docs` | Read the agent guide, list/show embedded documentation, or get the docs site URL |

The server also exposes **resources** (`fcheap://agent-guide`,
`fcheap://stashes`, `fcheap://stash/{id}`) and **prompts**
(`investigate_stash`, `find_across_stashes`)—see the
[MCP overview](/mcp/overview). Stash resources contain summaries and manifests,
not arbitrary saved file bodies.

## Tool Schemas

All tools use typed input structs with JSON schema auto-generation. The `jsonschema` tag provides the description, and `json` tag `omitempty` controls required vs optional.

```go
type saveInput struct {
    Path string `json:"path" jsonschema:"Absolute path to the file or directory"`
    Name string `json:"name,omitempty" jsonschema:"Display name for the stash"`
}
```
