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
- `source` (string, optional) -- original artifact this stash derives from (provenance)

**Output:** `{ manifest, secrets_warning?, secrets? }` -- the stash manifest, plus a
secrets warning and findings (file/rule/line) when the save-time scan flags likely
credentials.

### fcheap_list

List stashes, optionally filtered by tag, tool, and age. Newest first.

**Input:**
- `tag` (string, optional) -- filter by tag
- `tool` (string, optional) -- filter by tool (e.g. vidtrace)
- `since` (string, optional) -- only stashes newer than `24h`, `7d`, `2w`, or `2026-06-01`
- `limit` (int, optional) -- maximum number of stashes

**Output:** Array of stash summaries (id, name, tool, tags, sizes, created, bundle
type, plus compression / secrets_found / video where present)

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
- `limit` (int, optional) -- maximum number of results (default 20)
- `mode` (string, optional) -- `keyword`, `semantic`, or `hybrid` (default: `hybrid` if an embedder is configured, else `keyword`)

**Output:** Search results with scores and snippets, each naming the matching file

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

### fcheap_connect

Connect a stash to a codebase: run semantic code search (vecgrep) over the
codebase using the stashed artifact's text to surface the `file:line` candidates
most likely responsible for the bug. See [`connect`](/cli/connect).

**Input:**
- `stash_id` (string, required) -- the stash whose content drives the search
- `codebase_dir` (string, required) -- absolute path to the codebase
- `query` (string, optional) -- override the auto-extracted query
- `limit` (int, optional) -- max code matches (default 10)
- `index` (bool, optional) -- build the vecgrep index for the codebase first
- `mode` (string, optional) -- vecgrep search mode: `semantic`, `keyword`, or `hybrid` (default hybrid)

**Output:** Ranked code matches with `file:line`, score, and snippet

### fcheap_vacuum

Remove orphaned metadata- and search-index entries for stashes whose directory no
longer exists, then compact the database. See [`vacuum`](/cli/vacuum).

**Input:** none

**Output:** `{ on_disk, orphaned_rows, orphans }`

### fcheap_docs

Access fcheap documentation. Useful for agents that need to look up how a command works.

**Input:**
- `action` (string, required) -- `list`, `show`, or `site`
- `page` (string, optional) -- doc page path (for `action=show`), e.g., `cli/save`, `guide/getting-started`

**Output:** List of pages, page content, or site URL

## Architecture

The MCP server is built with the official [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk). Tools use typed input structs with `jsonschema` tags for automatic schema generation. The `DestructiveHint`, `OpenWorldHint`, and `IdempotentHint` annotations help AI assistants understand the safety properties of each tool.