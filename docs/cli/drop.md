# drop

Permanently delete a stash and all its files. This operation is irreversible.

## Usage

```bash
fcheap drop <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | `false` | Required to confirm deletion |

## Examples

```bash
# Drop a stash (requires --force)
fcheap drop my_artifacts_20260622_115254 --force
```

## Safety

The `--force` flag is required to prevent accidental deletion. Without it, fcheap will refuse to drop the stash and print a warning.

## Output

```
Dropped stash: my_artifacts_20260622_115254
```