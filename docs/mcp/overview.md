# MCP Server Overview

fcheap includes a built-in MCP (Model Context Protocol) server that exposes stash operations as tools for AI assistants like Claude.

## How It Works

The MCP server runs over stdio transport. When an AI assistant needs to save, restore, analyze, or diff files, it calls fcheap's MCP tools. The server creates a stash manager, performs the operation, and returns results as JSON.

## Setup

### Claude Code

Add to your MCP config:

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

### Other MCP Clients

Any MCP-compatible client that supports stdio transport can connect to `fcheap mcp serve`.

## Tools

### fcheap_save

Save a file or directory to the stash vault.

**Input:**
- `path` (string, required) -- absolute path to save
- `name` (string, optional) -- display name
- `tags` (string[], optional) -- categorization tags
- `tool` (string, optional) -- source tool (e.g., "vidtrace")

**Output:** Stash manifest as JSON

### fcheap_list

List all stashes, optionally filtered by tag.

**Input:**
- `tag` (string, optional) -- filter by tag

**Output:** Array of stash summaries

### fcheap_info

Get detailed information about a stash.

**Input:**
- `stash_id` (string, required) -- the stash ID

**Output:** Full manifest with file tree

### fcheap_restore

Restore a stash to a target directory.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `target` (string, optional) -- target directory (default: /tmp/<stash-id>)

**Output:** Restoration confirmation

### fcheap_drop

Permanently delete a stash.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `force` (bool, required) -- must be true to confirm

**Output:** Deletion confirmation

### fcheap_search

Search across all indexed stashes.

**Input:**
- `query` (string, required) -- search query

**Output:** Search results with scores and snippets

### fcheap_analyze

Index a stash for search and optionally search within it.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `query` (string, optional) -- search within the stash

**Output:** Index status, bundle type, and optional search results

### fcheap_diff

Compare a stash against a target directory.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `target_dir` (string, required) -- directory to compare against

**Output:** Diff result with file-level changes

### fcheap_docs

Access fcheap documentation. Useful for agents that need to look up how a command works.

**Input:**
- `action` (string, required) -- `list`, `show`, or `site`
- `page` (string, optional) -- doc page path (for `action=show`), e.g., `cli/save`, `guide/getting-started`

**Output:** List of pages, page content, or site URL

## Architecture

The MCP server is built with the official [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk). Tools use typed input structs with `jsonschema` tags for automatic schema generation. The `DestructiveHint`, `OpenWorldHint`, and `IdempotentHint` annotations help AI assistants understand the safety properties of each tool.