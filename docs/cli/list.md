# list

List saved stashes, optionally filtered by tag.

## Usage

```bash
fcheap list [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tag` | string | `""` | Filter by tag |
| `--tool` | string | `""` | Filter by tool (e.g. vidtrace) |
| `--since` | string | `""` | Only show stashes newer than `24h`, `7d`, `2w`, or `2026-06-01` |
| `--json` | bool | `false` | Output as JSON |

## Examples

```bash
# List all stashes (newest first)
fcheap list

# Filter by tag or tool
fcheap list --tag OPG-15061
fcheap list --tool vidtrace

# Only stashes from the last day / week
fcheap list --since 24h
fcheap list --since 7d

# JSON output (for scripting)
fcheap list --json
```

## Output

A table sorted newest-first, with colored compression (`zst`/`gz`) and `⚠ secrets`
indicators where applicable:

```
Stashes (3)

ID                         TOOL      TAGS        FILES  SIZE     AGE       COMP
opg_15061_20260622         vidtrace  bug,login   805    45.2 MiB  2h ago   zst
config_snap_20260622       -         config      1      2.1 KiB   5h ago   -
logs_20260622              -         logs        42     1.2 MiB   1d ago   -
```