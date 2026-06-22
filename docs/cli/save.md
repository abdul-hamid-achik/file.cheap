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
| `--source` | string | `""` | Source path (e.g., original video file) |

## Examples

```bash
# Save a directory
fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace

# Save with multiple tags
fcheap save ./report.pdf --tag evidence --tag pdf --tool manual

# Save with source provenance
fcheap save /tmp/vidtrace-output --tag OPG-15061 --tool vidtrace --source ~/Downloads/OPG-15061.mp4

# Save a single file
fcheap save ./config.yaml --tag config
```

## What Happens

1. fcheap resolves the path to an absolute path
2. Creates a stash directory at `<stash-dir>/<stash-id>/`
3. Copies the file tree into `content/`
4. Generates a `manifest.json` with metadata, provenance, file count, size, and content hashes
5. Auto-detects bundle type (vidtrace, generic)
6. Prints the stash ID and summary

## Output

```
Saved stash: my_artifacts_20260622_115254
  Source: /tmp/artifacts
  Tool: vidtrace
  Bundle: vidtrace
  Files: 805
  Size: 45.2 MB
  Tags: [OPG-15061]
```