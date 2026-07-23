# Troubleshooting

Start with the built-in health check:

```bash
fcheap doctor
```

It checks the configuration, vault paths, local indexes, vecgrep, and the
configured embedder. Use `--log-level debug` on a failing command for operation
traces on stderr without contaminating normal or JSON output.

## The command cannot find the expected vault

Show the effective configuration and paths:

```bash
fcheap config path
fcheap config show
```

The default data root is `${XDG_DATA_HOME:-$HOME/.local/share}/fcheap`. The
`--stash-dir` flag and `FCHEAP_STASH_DIR` override the configured value.
Relative `stash_dir` and `vecgrep_path` values resolve against the directory
containing `config.yaml`, not the current working directory.

## Save rejects the source path

The source cannot be the vault, live inside the vault, or contain the vault.
This protects file.cheap from recursively saving its own data. A source-root
symlink is also rejected; pass its resolved path instead.

Use these commands to compare the paths you intended:

```bash
fcheap config get stash_dir
pwd
```

## Search returns no results

Saving does not index content unless you pass `--index`.

```bash
fcheap save ./evidence --index
# or, for an existing stash:
fcheap analyze <stash-id>
fcheap search "exact phrase" --mode keyword
```

An empty result when nothing is indexed is not a command failure. Start with a
distinctive identifier or literal error text, then try broader language.

Binary files may contribute metadata but not searchable body text. For
vidtrace, the recognized bundle must contain `metadata.json` and
`timeline.json`.

## Semantic or hybrid search behaves like keyword search

Without a reachable embedder, semantic and hybrid modes gracefully fall back to
keyword search. Check the configured values and runtime:

```bash
fcheap config get embedder
fcheap config get embed_model
fcheap doctor
```

For local Ollama, configure the actual key name `embedder`:

```bash
fcheap config set embedder ollama
fcheap config set embed_model nomic-embed-text
fcheap config set ollama_url http://127.0.0.1:11434
```

Re-run `analyze` after enabling or changing an embedding model so documents have
vectors built with the current profile.

## Remote indexing is blocked after a secret warning

OpenAI and non-loopback Ollama endpoints receive document text. If the
save-time scanner flagged a stash, file.cheap blocks remote indexing before
sending its content unless `allow_remote_secrets` is explicitly enabled.

Review the findings with:

```bash
fcheap info <stash-id>
```

Prefer removing the secret, saving a clean snapshot, or using loopback Ollama.
Set `allow_remote_secrets: true` only when you have deliberately accepted the
remote disclosure boundary. Search-query text is not scanned by this guard.

## Connect reports a missing index or cannot find vecgrep

`connect` requires the separately installed vecgrep executable. Confirm it is
available:

```bash
fcheap doctor
```

If vecgrep exists but the repository is not indexed, retry with:

```bash
fcheap connect <stash-id> /absolute/path/to/repository --index
```

Indexing writes vecgrep's derived index in the target repository. If the stash
has no readable text, provide a focused query with `--query`.

## Restore reports verification mismatches

Restore prints the result and exits nonzero when bytes do not match the
manifest. The restored files remain available for investigation.

Do not overwrite the stash or discard the mismatch. Compare the reported paths
with `fcheap info <stash-id>`. Use `--allow-mismatch` only when intentionally
recovering known-damaged data and when downstream automation can tolerate an
unverified result.

## A stash disappeared from list

Expired stashes are hidden by default:

```bash
fcheap list --include-expired
fcheap sweep
```

The first command shows expired entries. The second previews what an applied
sweep would remove. Expiry alone does not delete the payload.

## Disk use is higher than expected

Inspect logical and estimated stored size by producer:

```bash
fcheap ecosystem-status
fcheap cleanup
```

Both commands are read-only by default. Compress a large stash, assign TTLs to
regenerable data, or review cleanup recommendations before applying deletion.
Use `fcheap vacuum` only to repair and compact derived indexes; it does not
remove a valid stash.

## An MCP client cannot start file.cheap

GUI clients often inherit a different `PATH` from your shell. Use the absolute
path returned by `command -v fcheap` in the client configuration, then run:

```bash
fcheap doctor
fcheap mcp serve
```

The MCP server waits on standard input when healthy. Stop the manual test with
Ctrl-C. See [MCP client setup](/integrations/mcp-clients) for client-specific
configuration.

## Get version-matched operating guidance

Run:

```bash
fcheap agent
fcheap docs list
fcheap docs show guide/troubleshooting
```

The first command prints the concise agent operating guide embedded in your
installed binary. The other commands list and read the complete embedded pages.
