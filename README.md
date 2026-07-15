# file.cheap

**The local artifact vault for coding agents.**

file.cheap snapshots the screenshots, logs, reports, transcripts, repro bundles,
and temporary folders that agent workflows create. Each stash gets provenance,
per-file hashes, searchable content, and a deliberate lifecycle—without requiring
an account or hosted service.

[Documentation](https://file.cheap/) ·
[Five-minute start](https://file.cheap/guide/getting-started) ·
[Agent guide](https://file.cheap/guide/agent-guide) ·
[MCP setup](https://file.cheap/integrations/mcp-clients)

## Why it exists

Agent work leaves useful evidence outside Git: a reproduction folder in `/tmp`,
a generated report, a directory of frames, or logs from a debugging session.
Those files are easy to create and surprisingly hard to find, verify, and reuse.

file.cheap gives them:

- a stable stash ID;
- source, tool, tag, size, and retention metadata;
- SHA-256 hashes and verified restores;
- local BM25 search, with optional semantic and hybrid modes;
- a CLI, terminal Studio, and local stdio MCP server;
- explicit compression, TTL, sweep, cleanup, and deletion.

It is not cloud sync, consumer backup, or a replacement for Git. The current
product stores its vault on your machine.

## Install

### macOS

```bash
brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap
```

### Linux (Debian/Ubuntu, amd64)

```bash
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/abdul-hamid-achik/file.cheap/releases/latest)"
tag="${tag##*/}"
version="${tag#v}"
curl -fLO "https://github.com/abdul-hamid-achik/file.cheap/releases/download/${tag}/fcheap_${version}_linux_amd64.deb"
sudo dpkg -i "fcheap_${version}_linux_amd64.deb"
```

RPM and arm64 packages are available on
[GitHub Releases](https://github.com/abdul-hamid-achik/file.cheap/releases/latest).

### From source

```bash
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

## First stash

Run a complete save → search → inspect → restore workflow:

```bash
# Check the local paths and optional integrations.
fcheap doctor

# Save and index any file or directory.
fcheap save ./agent-artifacts --tag bug-142 --tool my-agent --index

# Copy the returned stash ID, then find content across indexed stashes.
fcheap search "columns disappeared after refresh"
fcheap info <stash-id>

# Restore to a fresh temporary directory and verify every hash.
fcheap restore <stash-id>
```

The manifest stays the portable source of truth. SQLite and veclite are derived
local indexes that file.cheap can rebuild.

## Give an agent the vault

Ask the installed binary for its version-matched operating contract:

```bash
fcheap agent
fcheap agent --json
```

Start the local MCP server with:

```bash
fcheap mcp serve
```

For Claude Code:

```bash
claude mcp add -s user fcheap -- fcheap mcp serve
```

For Codex CLI, add:

```toml
[mcp_servers.fcheap]
command = "fcheap"
args = ["mcp", "serve"]
```

The server exposes typed tools, stash resources, reusable investigation prompts,
and the static `fcheap://agent-guide` resource.

## Search and privacy boundaries

Keyword indexing and BM25 search stay local. Semantic and hybrid search require an
embedder:

- loopback Ollama keeps document and query text on the machine;
- OpenAI or non-loopback Ollama receives indexed text and semantic queries;
- save-time secret findings block remote indexing unless you explicitly opt in;
- secret scanning is a warning system, not proof that content is safe to send.

The MCP server also runs locally, but your MCP client may send tool results to its
configured model provider. Treat saved artifact text as untrusted input.

## Core commands

| Job | Commands |
|---|---|
| Capture and inspect | `save`, `list`, `info` |
| Find and investigate | `analyze`, `search`, `diff`, `connect` |
| Recover | `restore` |
| Manage storage | `compress`, `ttl`, `sweep`, `cleanup`, `vacuum` |
| Work interactively | `studio` |
| Connect agents | `agent`, `mcp serve`, `docs` |

`connect` uses an optional, separately installed vecgrep binary and returns
ranked source-code candidates—not proof of code ownership.

## Develop

Requirements:

- Go 1.25+
- Bun 1.3+ for the VitePress site

```bash
go test ./...
go build ./cmd/fcheap

cd docs
bun install --frozen-lockfile
bun run docs:verify
```

See the [core concepts](https://file.cheap/guide/core-concepts), the
[CLI reference](https://file.cheap/cli/), and
[contribution guidance](AGENTS.md) for the complete architecture and conventions.

## License

MIT
