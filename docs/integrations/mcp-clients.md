# Connect file.cheap to MCP clients

file.cheap is a local stdio Model Context Protocol server. Your MCP client
launches `fcheap mcp serve` as a child process and communicates through standard
input and output. The same 14 tools, three resource shapes, two prompts, and
agent operating instructions are available in every compatible client.

Install file.cheap and run `fcheap doctor` before adding it to a client. The
`fcheap` executable must be on the environment path inherited by the client.
You can inspect the operating guide supplied to clients with `fcheap agent`.

## Claude Code

Register the server at user scope:

```bash
claude mcp add -s user fcheap -- fcheap mcp serve
```

The equivalent JSON shape is:

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

## Codex CLI

Add this block to `~/.codex/config.toml`:

```toml
[mcp_servers.fcheap]
command = "fcheap"
args = ["mcp", "serve"]
```

Restart the client after changing its configuration so it can discover the
server and its capabilities.

## OpenCode

Add a local server under `mcp` in
`~/.config/opencode/opencode.json`:

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

## Any stdio MCP client

Create a server named `fcheap` with:

| Setting | Value |
|---|---|
| Transport | stdio |
| Command | `fcheap` |
| Arguments | `mcp`, `serve` |
| Working directory | Any directory |

Clients that use an `mcpServers` JSON map can reuse the Claude Code JSON above.
Do not configure an HTTP URL: file.cheap intentionally does not run an API
server.

## What the client discovers

The server advertises all three MCP surfaces:

- **14 tools:** save, list, info, restore, drop, search, analyze, diff, connect,
  vacuum, TTL, sweep, cleanup, and embedded docs.
- **Resources:** `fcheap://agent-guide`, `fcheap://stashes`, and
  `fcheap://stash/{id}` for a single manifest.
- **Prompts:** `investigate_stash` and `find_across_stashes`.
- **Initialization instructions:** the concise, version-matched agent operating
  guide.

Keep the [MCP tools cheat sheet](/learn/mcp-tools-cheat-sheet) nearby when
designing an agent workflow. The complete schemas and safety behavior are in the
[MCP server reference](/mcp/overview).

The stash resources expose summaries and manifests. They do not expose
arbitrary saved file bodies. Search returns matching snippets; restore to a
fresh directory when complete content is needed and filesystem writes are in
scope.

## A safe first test

Ask the client to read `fcheap://agent-guide`, then perform a read-only
operation:

> List my five newest file.cheap stashes and show their IDs, tools, tags, and
> sizes. Do not restore or delete anything.

Then test a disposable save:

```bash
mkdir -p /tmp/fcheap-mcp-test
printf 'MCP connection test\n' > /tmp/fcheap-mcp-test/note.txt
```

Ask the agent to save `/tmp/fcheap-mcp-test` with tag `mcp-test`, inspect the
returned manifest, and report the stash ID. Ask it to delete the disposable
stash only after you explicitly approve the drop.

## Troubleshooting

### The client cannot find `fcheap`

GUI applications may inherit a different `PATH` than your shell. Use the
absolute path returned by `command -v fcheap` as the configured command, or
launch the client from an environment where the binary is available.

### The server exits immediately

Run these commands in a terminal:

```bash
fcheap doctor
fcheap mcp serve
```

The second command waits for MCP traffic on stdin; stop it with Ctrl-C. Any
configuration or vault error printed before that point explains why the client
could not keep the process alive.

### Search returns no results

Saving and indexing are separate unless `index: true` is passed to
`fcheap_save`. Call `fcheap_analyze` for the stash, then retry `fcheap_search`.
BM25 needs no external model.

### Connect is unavailable

`fcheap_connect` requires a separately installed vecgrep binary. Run
`fcheap doctor` and see the [`connect` setup](/cli/connect#requirements).
