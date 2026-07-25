# save

Save a file or directory to the stash vault.

## Usage

```bash
fcheap save <path> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | derived from path | Display name for the stash |
| `--tag` | string slice | `[]` | Tags for categorization (repeatable) |
| `--tool` | string | `""` | Tool that produced the content (e.g., vidtrace) |
| `--source` | string | `""` | Original artifact this stash derives from (provenance) |
| `--ttl` | string | `""` | Time-to-live (e.g. `7d`, `24h`, `30d`); empty = never expires |
| `--no-scan` | bool | `false` | Skip the save-time secret scan |
| `--no-compress` | bool | `false` | Skip auto-compression of large stashes |
| `--index` | bool | `false` | Index the stash for search immediately after saving (no separate `analyze` step) |

## Examples

```bash
# Save a directory
fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace

# Save with multiple tags
fcheap save ./report.pdf --tag evidence --tag pdf --tool manual

# Save with source provenance
fcheap save /tmp/vidtrace-output --tag EXAMPLE-1234 --tool vidtrace --source ~/Downloads/EXAMPLE-1234.mp4

# Mark the stash expired after 7 days; apply sweep separately to delete it
fcheap save /tmp/codemap-snapshot --tag codemap-snapshot --tool codemap --ttl 7d

# Save a single file
fcheap save ./config.yaml --tag config

# Save and index in one step — searchable immediately, no separate `analyze`
fcheap save /tmp/evidence --tag bug-42 --tool cortex --index
```

## What Happens

1. fcheap resolves and validates the source path
2. Creates a stash directory at `<stash-dir>/<stash-id>/`
3. Copies the file tree into `content/`
4. Generates a `manifest.json` with metadata, provenance, file count, size, and content hashes
5. Auto-detects bundle type (vidtrace, generic)
6. Scans content for likely secrets unless you pass `--no-scan`
7. With `--index`, indexes the stash for search and records `custom.indexed`
8. Prints the generated stash ID and summary

## Path and ID safety

The source must not overlap the stash vault. fcheap rejects the vault itself, a
path inside the vault, and any parent directory that contains the vault. It also
rejects a source-root symlink; pass the resolved path instead. These checks use
canonical paths so symlinks cannot bypass them.

Stash IDs are generated, bounded, and collision-resistant. Treat an ID as an
opaque single path element and copy it from `save` or `list`; fcheap rejects path
separators and traversal values in every ID-taking operation.

## Secret scanning

On save, fcheap scans text files for likely credentials — AWS/GitHub/Slack/Google
keys, private keys, JWTs, and generic `key = secret` assignments. It records the
finding count and rule names in the manifest and reports **file, rule, and line**
without exposing the secret value. Use `--no-scan` to skip, or review with
[`info`](/cli/info). A `⚠ secrets` chip also appears in
[Studio](/studio/overview).

If you configure OpenAI or a non-loopback Ollama endpoint, flagged stashes are
blocked from remote indexing unless you explicitly set
`allow_remote_secrets: true`. Loopback Ollama remains local and exempt. With
`save --index`, a policy block leaves the stash safely saved, emits
`status: "saved_with_failures"` with an `index` failure, and exits nonzero; you
can review it before running `analyze` again. See
[`config`](/cli/config#remote-embedding-safety).

## Output

```
Saved stash: <generated-stash-id>
  Source: /tmp/artifacts
  Tool: vidtrace
  Bundle: vidtrace
  Files: 805
  Size: 45.2 MB
  Tags: [EXAMPLE-1234]
! 2 potential secret(s) detected in this stash — review before sharing or restoring elsewhere
  └─ .env:1 [aws-access-key]
  └─ config.yaml:7 [generic-secret]
```

With `--json`, manifest fields remain at the root for compatibility and the
result adds `status`, `index_requested`, `indexed`,
`auto_compression_requested`, `auto_compressed`, and a stable `failed` array.
Indexing or automatic-compression failures produce
`status: "saved_with_failures"` and a nonzero exit after the successfully saved
manifest has been printed.
