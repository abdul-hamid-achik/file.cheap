# CLI reference

The `fcheap` binary manages a local vault of file and directory snapshots. It
prints human-readable output by default and stable JSON where `--json` is
supported.

## Usage

```bash
fcheap [global flags] <command> [arguments] [flags]
```

Run `fcheap <command> --help` for the flags accepted by the installed version.
Run `fcheap agent` for the embedded agent operating guide.

## Commands by job

### Save and inspect

| Command | Purpose |
|---|---|
| [`save`](/cli/save) | Snapshot a file or directory with provenance, hashes, and secret scanning |
| [`artifact-ref`](/cli/artifact-ref) | Emit versioned metadata that points to an existing local stash |
| [`list`](/cli/list) | Browse and filter stash summaries |
| [`info`](/cli/info) | Read one complete stash manifest |
| [`restore`](/cli/restore) | Materialize a stash and verify its hashes |
| [`drop`](/cli/drop) | Permanently delete one stash with explicit confirmation |

### Search and compare

| Command | Purpose |
|---|---|
| [`analyze`](/cli/analyze) | Index one stash for per-file search |
| [`search`](/cli/search) | Search indexed stashes with keyword, semantic, or hybrid retrieval |
| [`diff`](/cli/diff) | Compare a saved tree with a corresponding current directory |
| [`connect`](/cli/connect) | Use optional vecgrep to rank source code related to saved evidence |

### Storage lifecycle

| Command | Purpose |
|---|---|
| [`compress`](/cli/compress) | Archive a stash with zstd, gzip, or uncompressed tar |
| [`ttl`](/cli/ttl) | Set, update, or clear expiry metadata |
| [`sweep`](/cli/sweep) | Preview or apply removal of expired stashes |
| [`cleanup`](/cli/cleanup) | Score or categorize cleanup candidates |
| [`vacuum`](/cli/vacuum) | Remove orphaned derived entries and compact SQLite |
| [`ecosystem-status`](/cli/ecosystem-status) | Summarize storage and cleanup estimates by producing tool |

### Configuration and interfaces

| Command | Purpose |
|---|---|
| [`config`](/cli/config) | Inspect and update configuration |
| [`doctor`](/cli/doctor) | Check paths, indexes, and optional dependencies |
| [`studio`](/cli/studio) | Open the interactive terminal interface |
| [`mcp`](/cli/mcp) | Start the stdio MCP server |
| [`docs`](/cli/docs) | Read embedded documentation or manage the docs site from a checkout |
| `agent` | Print the version-matched agent operating guide; add `--json` for its structured form |
| [`completion`](/cli/completion) | Generate shell completion scripts |
| [`version`](/cli/version) | Print version, commit, and build date |

## Global flags

Global flags can appear before or after the command:

| Flag | Default | Description |
|---|---|---|
| `--json` | `false` | Print structured JSON when the command supports it |
| `--quiet` | `false` | Suppress non-error human output |
| `--no-color` | `false` | Disable color; file.cheap also honors `NO_COLOR` |
| `--stash-dir <path>` | configured value | Override the stash vault for this invocation |
| `--log-level <level>` | configured value | Set `debug`, `info`, `warn`, or `error` logging on stderr |
| `--help` | — | Show command help |
| `--version` | — | Print the root command version string |

## A complete first workflow

Saving and indexing are separate unless you request both:

```bash
fcheap save ./evidence --tag bug-123 --tool manual --index
fcheap search "checkout stopped" --mode keyword
fcheap info <stash-id>
fcheap restore <stash-id>
```

Copy `<stash-id>` from the save result. The restore command creates a fresh
temporary directory when `--to` is omitted and reports whether every file
matched its manifest hash.

## Automation contract

Use `--json` rather than parsing colored tables. Operational logs and debug
traces go to stderr, while command results go to stdout.

Some commands can complete their primary operation and still report a partial
failure. For example, a snapshot can be saved even if requested indexing fails,
and a payload can be deleted even if derived-index cleanup fails. Inspect
`status` and `failed` fields as well as the process exit code.

An empty search result or missing vecgrep index can be valid data rather than a
runtime error. The relevant command pages document those cases.

## Local and remote boundaries

The vault, manifest database, and BM25 index stay on the machine. A configured
OpenAI or non-loopback Ollama embedder receives text during semantic/hybrid
indexing and search. `connect` executes the separately installed vecgrep binary.

No command in this reference uploads stashes to a shipped file.cheap cloud
service; cloud sync is not part of the current product.

`artifact-ref` lets another tool store a pointer to a local stash. The pointer
does not move bytes and resolves only where the referenced vault is available.
See [Local artifact references for agent tools](/integrations/local-artifact-references).

## More help

- [Getting started](/guide/getting-started)
- [Core concepts](/guide/core-concepts)
- [Agent operating guide](/guide/agent-guide)
- [Troubleshooting](/guide/troubleshooting)
