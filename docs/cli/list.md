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
| `--json` | bool | `false` | Output as JSON |

## Examples

```bash
# List all stashes
fcheap list

# Filter by tag
fcheap list --tag OPG-15061

# JSON output (for scripting)
fcheap list --json
```

## Output

```
Stashes (3)

  ID                                 Name             Tool       Files  Size     Created
  my_artifacts_20260622_115254       my artifacts     vidtrace   805    45.2 MB  2026-06-22
  config_snap_20260622_100000        config snap      manual     1      2.1 KB   2026-06-22
  logs_20260622_090000               logs             manual     42     1.2 MB   2026-06-22
```