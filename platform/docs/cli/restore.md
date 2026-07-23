# restore

Restore a stash to a target directory and verify the restored files against the
manifest hashes.

## Usage

```bash
fcheap restore <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--to` | string | a fresh temp dir | Target directory for extraction |
| `--allow-mismatch` | bool | `false` | Exit successfully even when hash verification fails |

With no `--to`, restore creates a fresh, unique temp directory (e.g.
`$TMPDIR/<stash-id>-XXXXXX`) rather than a predictable shared path — so repeated
restores never merge into one another, and nothing can be pre-planted at a known
destination. The chosen directory is reported in the output.

The target must neither be inside the stash vault nor contain it. fcheap checks
canonical paths, so symlink aliases and parent directories of the vault are
rejected too.

::: warning Existing targets are modified
When `--to` names an existing directory, restore merges into it and replaces
same-named files. Unrelated files already in that directory remain. Use the
default fresh temp directory or an empty destination when you do not want an
existing tree modified.
:::

## Examples

```bash
# Restore to a specific directory
fcheap restore <stash-id> --to /tmp/working/

# Restore to a fresh temp directory (path is printed)
fcheap restore <stash-id>

# Recover known-damaged content but accept a verification mismatch
fcheap restore <stash-id> --to /tmp/forensics/ --allow-mismatch
```

## What Happens

1. If the stash is compressed (`.tar.zst`), extracts the archive
2. If the stash is a plain tree, copies the files
3. Creates the target directory if it doesn't exist
4. Preserves file permissions and directory structure
5. Hashes the restored files and compares them with `manifest.json`

## Verification contract

Restored bytes and integrity verification are one command contract. If any hash
does not match, fcheap prints the complete result and exits nonzero by default.
The restored files remain available for inspection. Pass `--allow-mismatch` only
when you intentionally accept that result, such as forensic recovery.

With `--json`, automation receives the result before the exit status:

```json
{
  "stash_id": "<stash-id>",
  "target": "/tmp/working",
  "file_count": 805,
  "verified": false,
  "mismatches": ["logs/app.log (hash mismatch)"],
  "status": "restored_with_mismatches"
}
```

`status` is `restored`, `restored_unverified`, or
`restored_with_mismatches`. Copy stash IDs from `save` or `list`; IDs are opaque
single path elements, and path-like or traversal IDs are rejected.
