# list

List saved stashes, optionally filtered by tag.

## Usage

```bash
fcheap list [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tag` | string (repeatable) | `[]` | Filter by tag — AND across repeats (a stash must contain **every** listed tag). Comma-separated values accepted. |
| `--tool` | string | `""` | Filter by tool (e.g. vidtrace) |
| `--since` | string | `""` | Only show stashes newer than `24h`, `7d`, `2w`, or `2026-06-01` |
| `--include-expired` | bool | `false` | Include expired stashes (hidden by default) |
| `--json` | bool | `false` | Output as JSON |

## Examples

```bash
# List active stashes (newest first)
fcheap list

# Filter by tag or tool
fcheap list --tag EXAMPLE-1234

# Filter by multiple tags (AND — stash must contain all of them).
# Used by codemap's per-branch index cache: list codemap snapshots for one repo.
fcheap list --tag codemap-index --tag repo:abc123
fcheap list --tool vidtrace

# Only stashes from the last day / week
fcheap list --since 24h
fcheap list --since 7d

# JSON output (for scripting). Each item carries id, name, tool, tags, file_count,
# total_size, content_hash, bundle_type, compression, compressed_size, expires_at,
# created_at and custom (including --meta values). See "JSON output" below.
fcheap list --json

# Include expired stashes (past their TTL)
fcheap list --include-expired
```

## Output

A table sorted newest-first, with colored compression (`zst`/`gz`) indicators
where applicable (the Studio TUI additionally shows a `⚠ secrets` chip):

```
Stashes (3)

ID                         TOOL      TAGS        FILES  SIZE     AGE       EXP    COMP
example_1234_20260622      vidtrace  bug,login   805    45.2 MiB  2h ago   -      zst
config_snap_20260622       -         config      1      2.1 KiB   5h ago   7d     -
logs_20260622              -         logs        42     1.2 MiB   1d ago   -      -
```

The `EXP` column shows the remaining TTL (e.g. `7d`, `12h`) or `EXPIRED` when past. A `-` means no TTL (permanent).

## JSON output

`fcheap list --json` prints an array. Keys marked optional are omitted when empty.

| Field | Meaning |
|---|---|
| `id` | Stash ID |
| `name`, `tool`, `tags` | Optional display name, producing tool, and tags |
| `file_count`, `total_size` | Files in the stash and their logical size in bytes |
| `content_hash` | Digest of the content (optional) |
| `bundle_type` | Detected bundle type, such as `generic`, `vidtrace`, `monitor.incident`, `cairntrace-run`, or `glyphrun-run` (optional). Filter run bundles with it instead of calling `info` per stash |
| `compression` | `zstd`, `gzip`, or `none` once compressed (optional) |
| `compressed_size` | On-disk archive size in bytes once compressed (optional; omitted for an uncompressed stash). Compare with `total_size` for the saving |
| `expires_at`, `created_at` | RFC 3339 timestamps (`expires_at` is optional) |
| `custom` | Manifest custom fields, including `--meta` values and the scan accounting `secrets_files_scanned` / `secrets_files_skipped` (see [`save`](/cli/save#secret-scanning)) |

`bundle_type` and `compressed_size` are additive; existing consumers that ignore
unknown keys are unaffected.
