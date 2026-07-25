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

# JSON output (for scripting)
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
