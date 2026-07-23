# compress

Compress a stash to save disk space using tar+zstd archiving.

## Usage

```bash
fcheap compress <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--algo` | string | `zstd` | Compression algorithm (`zstd`, `gzip`, or `none` for an uncompressed tar) |

## Examples

```bash
# Compress with default zstd
fcheap compress my_artifacts_20260622_115254

# Compress with gzip
fcheap compress my_artifacts_20260622_115254 --algo gzip
```

## What Happens

1. Creates a `content.tar.zst` (or `content.tar.gz`) archive from the stash's `content/` directory
2. Removes the extracted `content/` directory
3. Updates the manifest with compression info and compressed size
4. The stash can still be restored -- fcheap extracts on demand

## Auto-compression

When saving, stashes larger than the `compress_threshold` config value (default 10MB) are automatically compressed.

## Output

```
Compressed stash: my_artifacts_20260622_115254
  Algorithm: zstd
  Archive: /home/user/.local/share/fcheap/my_artifacts_20260622_115254/content.tar.zst
  Compressed Size: 12.3 MB
```