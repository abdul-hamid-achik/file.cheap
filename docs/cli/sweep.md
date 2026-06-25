# sweep

Find and optionally drop stashes whose TTL has expired. By default sweep is a dry-run: it reports which stashes would be dropped without touching them. Use `--apply` to actually delete expired stashes.

## Usage

```bash
fcheap sweep [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--apply` | bool | `false` | Actually drop expired stashes (default: dry-run) |
| `--keep-tag` | string | `keep` | Tag that exempts a stash from sweeping |
| `--include-tag` | string | — | Only sweep stashes with this tag |

## Examples

```bash
# Dry-run: see which stashes would be swept
fcheap sweep

# Actually drop expired stashes
fcheap sweep --apply

# Only sweep codemap snapshots (regenerable cache)
fcheap sweep --apply --include-tag codemap-snapshot

# Use a custom keep tag
fcheap sweep --apply --keep-tag pinned
```

## Safety

- Stashes with the `keep` tag (configurable via `--keep-tag`) are **never swept**, even if their TTL has expired. This is a safety net for pinning important stashes.
- Sweep cleans up DB rows and search index entries for dropped stashes, matching the `drop` and `vacuum` patterns.

## Output

```

Sweep DRY RUN: 3 expired stash(es)
  └─ codemap_snapshot_20260615_103022
  └─ vecgrep_index_20260610_140533
  └─ temp_artifacts_20260601_091544

! This was a dry-run. Use --apply to actually drop these stashes.
```

## See also

- [`ttl`](/cli/ttl) — set a TTL on a stash
- [`cleanup`](/cli/cleanup) — smart heuristic analysis for dropping stashes
- [`list --include-expired`](/cli/list) — show expired stashes