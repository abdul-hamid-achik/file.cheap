# Start here

file.cheap gives files produced during agent work a durable local lifecycle. It
can snapshot an arbitrary file or folder, record where it came from, search its
contents later, and restore the exact saved bytes with hash verification.

The vault is local by default. You do not need an account or hosted service to
save, list, search, or restore a stash.

## Choose a path

| Goal | Start with |
|---|---|
| Install file.cheap and save something useful | [Getting started](/guide/getting-started) |
| Understand manifests, indexes, and the local storage model | [Core concepts](/guide/core-concepts) |
| Follow complete CLI and agent workflows | [Workflow examples](/guide/workflows) |
| Give an AI assistant safe operating instructions | [Agent guide](/guide/agent-guide) |
| Share saved artifacts with Chalupa, Cairntrace, or Glyphrun | [Local artifact references](/integrations/local-artifact-references) |
| Connect Claude Code, Codex CLI, or another MCP client | [MCP client setup](/integrations/mcp-clients) |
| Diagnose an installation or operation | [Troubleshooting](/guide/troubleshooting) |
| Look up a command or global flag | [CLI reference](/cli/) |

## The core loop

Most workflows use the same sequence:

```text
save -> index -> search or inspect -> restore when needed -> retain or remove
```

You can combine the first two steps with `fcheap save --index`. A successful
save returns an opaque stash ID. Keep that ID in an issue, agent transcript, or
investigation note so another operation can address the same snapshot.

```bash
fcheap save ./evidence --tag bug-123 --tool manual --index
fcheap search "checkout stopped"
fcheap info <stash-id>
fcheap restore <stash-id>
```

Search returns matching saved files and snippets. Restore without `--to`
creates a fresh temporary directory and verifies the restored files against the
manifest.

## What ships today

file.cheap is a local-first CLI, MCP server, and terminal UI. The installed Go
binary includes:

- file and folder snapshots with provenance and tags;
- save-time secret scanning;
- streaming compression and verified restore;
- local BM25 search, with optional semantic and hybrid search;
- comparison against a corresponding live directory;
- optional vecgrep integration for connecting evidence to likely source code;
- retention, cleanup planning, and a Studio terminal interface;
- embedded documentation and an operating guide for agents.

The installed product does not provide cloud sync, accounts, an HTTP API,
authentication, or billing. file.cheap also has a public product website and a
gated recovery laboratory, but no hosted vault ships to users. Optional OpenAI
and non-loopback Ollama embedders can receive text when you explicitly configure
them; the stash payload itself remains in the local vault.

## Next step

Follow [Getting started](/guide/getting-started) for an executable first stash,
search, and verified restore.
