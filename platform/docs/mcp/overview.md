# MCP Server Overview

fcheap includes a built-in MCP (Model Context Protocol) server that exposes
local stash operations to AI assistants across all three MCP surfaces:
**tools** (actions), **resources** (readable data by URI), and **prompts**
(guided workflows). It also supplies a version-matched operating guide during
initialization so a client learns the safety contract before choosing tools.

## How It Works

The MCP server runs over stdio transport. When an assistant needs to save,
restore, analyze, or compare files, it calls a typed fcheap tool. Agents can
also read stash metadata and the operating guide as resources, then launch
multi-step investigations from prompts.

The installed vault and MCP server are local. OpenAI and non-loopback Ollama
embedders are the exception: when explicitly configured, they receive text
during semantic/hybrid indexing and search. The public file.cheap website and
its private artifact service are separate; this MCP server exposes neither a
hosted HTTP API nor shipped cloud sync. A local MCP process also does not imply a
local language model: the client may send tool and resource results to its
configured model provider.

## Setup

fcheap is a standard **stdio** MCP server, so it works in any MCP-compatible
client. The invocation is always the same — command `fcheap`, args `mcp serve` —
only the config format differs. Below are the common clients.

Before connecting a client, inspect the same guide that the server will provide:

```bash
fcheap agent
fcheap agent --json
```

### Claude Code

One-liner (user scope, available in every project):

```bash
claude mcp add -s user fcheap -- fcheap mcp serve
```

Or add it to your MCP config by hand:

```json
{
  "mcpServers": {
    "fcheap": {
      "command": "fcheap",
      "args": ["mcp", "serve"]
    }
  }
}
```

### Codex CLI

In `~/.codex/config.toml`:

```toml
[mcp_servers.fcheap]
command = "fcheap"
args = ["mcp", "serve"]
```

### OpenCode

In `~/.config/opencode/opencode.json`, under `mcp`:

```json
{
  "mcp": {
    "fcheap": {
      "type": "local",
      "command": ["fcheap", "mcp", "serve"],
      "enabled": true
    }
  }
}
```

### Any other stdio MCP client

Register a server named `fcheap` that runs `fcheap mcp serve` over stdio. Clients
that read a `.mcp.json` (`mcpServers` map) use the same shape as the Claude Code
JSON above. On first connect the server advertises 15 tools, the
`fcheap://agent-guide` and `fcheap://stashes` resources, the
`fcheap://stash/{id}` resource template, and two prompts. Its initialization
instructions include the concise agent guide.

## Tools

### fcheap_save

Save a file or directory to the stash vault.

**Input:**

- `path` (string, required) -- absolute path to save
- `name` (string, optional) -- display name
- `tags` (string[], optional) -- categorization tags
- `tool` (string, optional) -- source tool (e.g., "vidtrace")
- `source` (string, optional) -- original artifact this stash derives from (provenance)
- `ttl` (string, optional) -- retention duration or date; empty means no expiry
- `index` (bool, optional) -- index immediately after saving

**Output:** `{ manifest, secrets_warning?, secrets? }` -- the stash manifest, plus a
secrets warning and findings (file/rule/line) when the save-time scan flags likely
credentials. With `index: true`, the result also contains `indexed` or
`index_error`; an indexing policy block never rolls back a successful save.

The path must be outside the stash vault and must not contain it. Canonical-path
checks also reject symlink-based overlap. Stash IDs are opaque single path
elements; all ID-taking tools reject separators and traversal values.

### fcheap_list

List active stashes, optionally filtered by tag, tool, and age. Newest first;
expired stashes are hidden unless requested.

**Input:**
- `tag` (string, optional) -- filter by tag (single; merged with `tags`)
- `tags` (string[], optional) -- filter by tags, AND across entries (a stash must contain every tag). Repeatable on the CLI: `--tag a --tag b`.
- `tool` (string, optional) -- filter by tool (e.g. vidtrace)
- `since` (string, optional) -- only stashes newer than `24h`, `7d`, `2w`, or `2026-06-01`
- `limit` (int, optional) -- maximum number of stashes
- `include_expired` (bool, optional) -- include expired stashes (default `false`)

**Output:** Array of stash summaries (id, name, tool, tags, sizes, created, bundle
type, plus compression / secrets_found / video / `custom` where present). `custom`
carries the full `manifest.Custom` map (e.g. `source` base-sha, `branch`,
`embedding_profile`), so a caller can rebuild a pointer file from `list` alone.

### fcheap_info

Get detailed information about a stash.

**Input:**
- `stash_id` (string, required) -- the stash ID

**Output:** Full manifest with file tree

### fcheap_artifact_ref

Return a stable ArtifactRefV1 for an existing local stash. This read-only,
idempotent tool does not upload, sign, restore, or mutate the stash.

**Input:**

- `stash_id` (string, required) -- existing local stash ID
- `kind` (string, optional) -- lowercase artifact kind; defaults to a safe kind
  derived from the manifest bundle type
- `producer_tool` (string, optional) -- native producer name; required when any
  other producer field is supplied
- `producer_version` (string, optional) -- producer version
- `native_schema` (string, optional) -- absolute schema URI without credentials
  or a query string
- `native_id` (string, optional) -- producer-native artifact ID
- `entrypoint` (string, optional) -- safe relative path to the native descriptor
  inside the stash; it must identify a regular saved file

**Output:** The strict JSON envelope with `$schema`,
`version: 1`, `provider: "fcheap-local"`,
`uri: "fcheap://stash/<artifact_id>"`, `artifact_id`, `kind`, and optional
`producer`. The same value is available as structured tool content.

For example, an agent attaching a Glyphrun pack can call:

```json
{
  "stash_id": "<stash-id>",
  "kind": "glyphrun.run",
  "producer_tool": "glyphrun",
  "native_schema": "urn:glyphrun.dev:run:v1",
  "native_id": "<raw-glyphrun-run-id>",
  "entrypoint": "run.json"
}
```

The structured result is the complete ArtifactRefV1 object. Pass that object
unchanged to the receiving integration; do not reduce it to a stash ID, invent
an `integrity` value, or replace its URI with a local filesystem path. For
Chalupa, place it in the artifact sidecar under the matching raw Cairn run ID
before the run's first ingest.

The emitted local envelope has no `integrity`, `web_url`, or dedicated
credential field. Its `fcheap://` URI is not signed and resolves only where the
matching local vault exists. Permitted identifiers and producer metadata are
not DLP-scanned. The tool returns an error instead of a reference when an
optional entrypoint is absent or is not a regular file. See
[`artifact-ref`](/cli/artifact-ref) and the
[ecosystem integration guide](/integrations/local-artifact-references).

### fcheap_restore

Restore a stash to a target directory.

**Input:**

- `stash_id` (string, required) -- the stash ID
- `target` (string, optional) -- target directory (default: a fresh, unique temp directory, reported in the result)
- `allow_mismatch` (bool, optional) -- accept an unverified restore (default `false`)

**Output:** `{ stash_id, target, file_count, verified, mismatches, status }`, where
`status` is `restored`, `restored_unverified`, or
`restored_with_mismatches`. When `verified` is false, the tool result has
`isError: true` unless `allow_mismatch` is true. The structured result remains
available either way. Restore targets that are inside the stash vault or contain
it, including through symlinks, are rejected. An existing target is modified in
place: same-named files are replaced and unrelated files remain. Omit `target`
for a fresh temp directory when replacement is not intended.

### fcheap_drop

Permanently delete a stash.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `force` (bool, required) -- must be true to confirm

**Output:** `{ stash_id, status, failed }`. `status` is `dropped` or
`dropped_with_failures`; a derived search-index cleanup failure remains in
`failed` and marks the tool result as an error even though stash deletion
succeeded.

### fcheap_search

Search across all indexed stashes.

**Input:**
- `query` (string, required) -- search query
- `limit` (int, optional) -- maximum number of results (default 20)
- `mode` (string, optional) -- `keyword`, `semantic`, or `hybrid` (default: `hybrid` if an embedder is configured, else `keyword`)

**Output:** Search results with scores and snippets, each naming the matching file

Semantic/hybrid mode sends query text to the configured HTTP embedder. OpenAI is
remote; Ollama defaults to localhost but may use a custom remote URL. Query text
is not covered by the save-time secret guard. In a mixed vault, semantic mode
also merges BM25 results from stashes that have no vectors.

### fcheap_analyze

Index a stash for search and optionally search within it.

**Input:**

- `stash_id` (string, required) -- the stash ID
- `query` (string, optional) -- search within the stash

**Output:** Index status, bundle type, and optional search results

With OpenAI or a non-loopback Ollama endpoint, indexing a stash flagged by the
save-time secret scanner is blocked unless `allow_remote_secrets: true` is
explicitly configured. Loopback Ollama remains local and exempt. An optional
query is sent to the configured embedder and is not scanned by that guard. See
[`config`](/cli/config#remote-embedding-safety).

### fcheap_diff

Compare a stash against a target directory.

**Input:**
- `stash_id` (string, required) -- the stash ID
- `target_dir` (string, required) -- directory to compare against

**Output:** Diff result with file-level changes

### fcheap_connect

Connect a stash to a codebase: run semantic code search (vecgrep) over the
codebase using the stashed artifact's text to rank related `file:line`
candidates for investigation. Matches are leads, not proof. See
[`connect`](/cli/connect).

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

### fcheap_ttl

Set or clear a stash expiry.

**Input:**

- `stash_id` (string, required) -- the stash ID
- `ttl` (string, required) -- duration/date, or an empty string to clear expiry

**Output:** `{ stash_id, expires_at }`

### fcheap_sweep

Plan or apply deletion of expired stashes.

**Input:**

- `apply` (bool, optional) -- delete the filtered plan (default `false`)
- `keep_tag` (string, optional) -- exempt this tag (default `keep`)
- `include_tag` (string, optional) -- include only stashes with this tag

`include_tag` is applied while building the plan, before mutation. The output
separates `expired` (planned IDs), `dropped` (successful deletions), and `failed`
(drop/index failures), along with `applied`, `skipped`, and `reclaimed`. A
non-empty `failed` array marks the tool result as an error.

### fcheap_cleanup

Analyze cleanup candidates in scoring mode or category-based smart mode.

**Input:**

- `apply`, `keep_tag` -- control mutation and protection
- `tool`, `tag`, `drop_only`, `expired` -- scoring-mode filters
- `smart`, `categories`, `stale_days` -- smart-mode controls

Both modes are dry-runs unless `apply` is true. Apply auto-deletes only expired
TTLs or `codemap`/`vecgrep` caches; missing-source and evidence recommendations
remain review-only. Smart output separates the pre-apply `analysis` from
`dropped`, `reclaimed`, `skipped`, and `failed`. A non-empty `failed` array marks
the tool result as an error. See [`cleanup`](/cli/cleanup).

### fcheap_docs

Access the read-only fcheap documentation embedded in every installed server.

**Input:**
- `action` (string, required) -- `guide`, `list`, `show`, or `site`
- `page` (string, optional) -- canonical embedded page path for `action=show`,
  e.g. `cli/save`; absolute and traversal paths are rejected

**Output:** The versioned agent guide, list of pages, exact page content, or site
URL. The `site` result notes that local VitePress serving requires a file.cheap
source checkout plus Bun; embedded `guide`, `list`, and `show` do not.

## Resources

Resources let an agent pull the operating contract or stash metadata directly
into context without an action-oriented tool call.

### `fcheap://agent-guide`

The version-matched, read-only operating guide. It explains tool selection,
state effects, remote embedding, restore verification, destructive-action
policy, and partial-failure handling. The server also includes this guide in its
initialization instructions.

### `fcheap://stashes`

The active stash index—the same default view as `fcheap_list`—with id, name,
tool, tags, file count, size, creation time, bundle type, and optional
compression, secret, video, and custom metadata. Expired stashes are hidden by
this default view.

### `fcheap://stash/{id}`

A resource **template**: read a single stash's full manifest by ID — provenance,
the file list with hashes, tags, compression, detected secrets, and bundle
metadata. Reading an unknown ID returns a resource-not-found error.

## Prompts

Prompts are reusable, parameterized workflows. They guide tool selection but do
not add capabilities beyond the tools and resources above.

### `investigate_stash`

Plan an end-to-end investigation of a stash. **Arguments:** `stash_id`
(required), `codebase_dir` (optional). It guides the agent through reading the
manifest, indexing and searching the stash, optionally connecting evidence to a
codebase, and summarizing supported findings.

### `find_across_stashes`

Search every indexed stash for a query and synthesize which stash and file has
the strongest evidence. **Arguments:** `query` (required), `mode` (optional:
`keyword`/`semantic`/`hybrid`). Search supplies snippets; the manifest resource
adds provenance and a file list, not surrounding file bodies. Restore to a fresh
directory when complete content is required and that write is in scope.

## Architecture

The MCP server is built with the official
[modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk).
Tools use typed input structs with `jsonschema` tags for schema generation. The
`DestructiveHint`, `OpenWorldHint`, and `IdempotentHint` annotations describe
tool behavior, while initialization instructions and `fcheap://agent-guide`
provide the operating policy. Mechanical annotations do not replace explicit
user intent for deletion or applied cleanup.
