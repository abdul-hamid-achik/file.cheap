# sweep

Plan and optionally apply TTL-based cleanup. By default sweep is a dry-run: it
reports eligible stashes without touching them. Use `--apply` to delete them.

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
| `--auto` | bool | `false` | Also plan safe smart-cleanup cache candidates; still requires `--apply` to delete |
| `--include-stale` | bool | `false` | With `--auto`, include the stale category in analysis |

## Examples

```bash
# Dry-run: see which stashes would be swept
fcheap sweep

# Actually drop expired stashes
fcheap sweep --apply

# Only sweep codemap snapshots (regenerable cache)
fcheap sweep --apply --include-tag codemap-snapshot

# Preview expired stashes and safe smart-cleanup cache candidates
fcheap sweep --auto

# Apply both parts of that plan
fcheap sweep --auto --apply

# Use a custom keep tag
fcheap sweep --apply --keep-tag pinned
```

## Safety

- `--auto` never implies `--apply`. It is a dry-run unless you pass both flags.
- The standard plan contains expired TTLs. The `--auto` extension can delete only
  `codemap`/`vecgrep` recommendations because those tools produce regenerable
  caches. Missing sources, evidence, duplicates, and other heuristic signals are
  not enough on their own.
- `--include-tag` filters both expired and auto candidates while the plan is
  built, before any mutation occurs.
- Stashes with the keep tag are never swept, even after their TTL expires.
- Sweep removes stash content through the normal drop path. Metadata and search
  index cleanup are best-effort; [`vacuum`](/cli/vacuum) repairs orphaned index
  entries.

## JSON contract

The nested `expired` result distinguishes the plan from what actually happened:

```json
{
  "expired": {
    "expired": ["planned-expired-id"],
    "dropped": [],
    "skipped": [],
    "failed": [],
    "applied": false,
    "reclaimed": 0
  },
  "auto": null,
  "auto_candidates": [],
  "auto_dropped": [],
  "auto_skipped": [],
  "auto_failed": [],
  "auto_reclaimed": 0
}
```

Inside that object, `expired` is the filtered plan, `dropped` contains successful
deletions, `skipped` contains unreadable manifests, and `failed` contains drop or
index failures. With `--auto`, `auto_candidates` is its safe cache plan,
`auto_dropped` is the successful subset, `auto_skipped` explains protections or
concurrent plan changes, and `auto_failed` records inspect, cancellation, drop,
or index failures. `reclaimed` and `auto_reclaimed` count only successful content
deletions. A non-empty `expired.failed` or `auto_failed` array makes the command
exit nonzero after printing JSON.

## See also

- [`ttl`](/cli/ttl) — set a TTL on a stash
- [`cleanup`](/cli/cleanup) — smart heuristic analysis for dropping stashes
- [`list --include-expired`](/cli/list) — show expired stashes
