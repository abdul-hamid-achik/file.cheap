# restore

Restore a stash to a target directory. Extracts all files from the stash.

## Usage

```bash
fcheap restore <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--to` | string | a fresh temp dir | Target directory for extraction |

With no `--to`, restore creates a fresh, unique temp directory (e.g.
`$TMPDIR/<stash-id>-XXXXXX`) rather than a predictable shared path — so repeated
restores never merge into one another, and nothing can be pre-planted at a known
destination. The chosen directory is reported in the output.

## Examples

```bash
# Restore to a specific directory
fcheap restore my_artifacts_20260622_115254 --to /tmp/working/

# Restore to a fresh temp directory (path is printed)
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