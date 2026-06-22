# restore

Restore a stash to a target directory. Extracts all files from the stash.

## Usage

```bash
fcheap restore <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--to` | string | `/tmp/<stash-id>` | Target directory for extraction |

## Examples

```bash
# Restore to a specific directory
fcheap restore my_artifacts_20260622_115254 --to /tmp/working/

# Restore to default location (/tmp/<stash-id>)
fcheap restore my_artifacts_20260622_115254
```

## What Happens

1. If the stash is compressed (`.tar.zst`), extracts the archive
2. If the stash is a plain tree, copies the files
3. Creates the target directory if it doesn't exist
4. Preserves file permissions and directory structure

## Output

```
Restored stash: my_artifacts_20260622_115254
  Target: /tmp/working/
  Files: 805
```