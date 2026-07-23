# cleanup

Analyze stashes for cleanup. Two modes: **scoring** (default) and **smart** (`--smart`).

## Usage

```bash
fcheap cleanup [flags]              # scoring mode (default)
fcheap cleanup --smart [flags]       # category-based smart mode
```

## Flags

### Common flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--apply` | bool | `false` | Apply the plan to eligible stashes (default: dry-run) |
| `--keep-tag` | string | `keep` | Tag that exempts a stash from cleanup |

### Scoring mode flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tool` | string | — | Only analyze stashes from this tool |
| `--tag` | string | — | Only analyze stashes with this tag |
| `--drop-only` | bool | `false` | Only show stashes scored as drop |
| `--expired` | bool | `false` | Include stashes with an expired TTL |

### Smart mode flags (`--smart`)

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--smart` | bool | `false` | Use category-based smart analysis |
| `--categories` | string | — | Filter to specific categories (comma-separated: expired,orphaned,superseded,duplicate,branch-gone,stale,keep) |
| `--stale-days` | int | `0` | Days without access to be considered stale (0 = disabled) |

## Scoring mode (default)

Scores each stash 0-100 on "droppability" using weighted signals:

| Signal | Weight | Description |
|--------|--------|-------------|
| Source path gone | +35 | The original source directory no longer exists |
| Cache tool (codemap, vecgrep) | +25 | Regenerable snapshots — safe to drop |
| Evidence tool (vidtrace, cairntrace) | -30 | Valuable evidence — protected |
| Old age (>90 days) | +15 | Stale, likely irrelevant |
| Large size (>100MB) | +10 | Reclaiming space matters more |
| Keep tag | -50 | User pinned this stash — protected (hard floor) |
| Expired TTL | +40 | Already past its intended lifetime |
| Content-hash dedup | +20 | An older stash with the same content as a newer one |

| Verdict | Score Range | Action with `--apply` |
|--------|-------------|---------------------|
| **drop** | >= 60 | Dropped only when its TTL expired or its tool is `codemap`/`vecgrep`; otherwise skipped for review |
| **review** | 30-59 | Not dropped |
| **keep** | < 30 | Not dropped |

## Smart mode (`--smart`)

Categorizes every stash into exactly one cleanup category based on why it might be droppable:

| Category | Description |
|----------|-------------|
| `expired` | TTL has elapsed |
| `orphaned` | The recorded source path no longer exists |
| `superseded` | A newer stash exists for the same tool + source path |
| `duplicate` | Same content hash as a newer stash |
| `branch-gone` | A `branch:` tag references a deleted git branch |
| `stale` | Older than `--stale-days` (uses `created_at` as proxy) |
| `keep` | No cleanup reason found |

Priority order ensures each stash gets only its first matching category (expired beats orphaned beats superseded, etc.) — no double counting.

`--categories` filters the analysis; it does not grant deletion permission. With
`--apply`, smart cleanup drops only:

- stashes whose TTL has expired, which reflects an explicit retention policy; or
- stashes produced by `codemap` or `vecgrep`, which are documented regenerable caches.

Other recommendations remain review-only. For example, a missing source,
superseded checkpoint, duplicate payload, deleted branch, or old age does not by
itself authorize deletion. The same rule protects evidence stashes such as
`vidtrace` and `cairntrace`; an explicit expired TTL still applies.

### Keep-tag protection

In both modes, stashes bearing the keep-tag (default: `keep`) are never dropped:

- **Scoring mode**: the keep-tag is a **hard floor** — the stash always gets a `keep` verdict regardless of its score. Even a cache-tool stash with expired TTL and source-gone will not be dropped if it has the keep tag.
- **Smart mode**: stashes with the keep-tag are skipped during `--apply` even if they're categorized as expired/orphaned/etc.

This is a safety net for pinning important stashes that should survive any cleanup.

## Examples

```bash
# Scoring mode: dry-run, see all stashes scored
fcheap cleanup

# Only show stashes scored as DROP (some may still require review)
fcheap cleanup --drop-only

# Apply the plan; only expired TTL or codemap/vecgrep candidates are deleted
fcheap cleanup --apply

# Analyze only codemap snapshots
fcheap cleanup --tool codemap

# Smart mode: categorize all stashes
fcheap cleanup --smart

# Smart mode: only show expired and orphaned stashes
fcheap cleanup --smart --categories expired,orphaned

# Apply expired TTLs; non-cache duplicates are reported as skipped
fcheap cleanup --smart --apply --categories expired,duplicate

# Smart mode: find stashes older than 30 days
fcheap cleanup --smart --stale-days 30
```

## Safety

- Both modes are dry-run by default.
- `--apply` auto-deletes only expired TTLs or `codemap`/`vecgrep` caches. A high
  score or smart category alone is not deletion consent.
- Stashes with the keep tag are never dropped. In scoring mode it forces a
  `keep` verdict; in smart mode a protected non-keep recommendation appears in
  `skipped` during apply.
- In smart mode, review `dropped`, `failed`, and `skipped` instead of assuming
  every recommendation was removed. Scoring mode exposes the same stable
  `dropped`, `failed`, `skipped`, and `reclaimed` result fields; either mode exits
  nonzero after printing JSON when `failed` is non-empty.

## JSON contract

In smart mode, `--json` separates the plan from the mutation result:

- `recommendations`, `by_category`, and `reclaimable` describe the analysis
  computed before apply.
- `applied` reports whether you passed `--apply`.
- `dropped` and `reclaimed` report successful deletions only.
- `failed` reports inspection or drop failures; the command exits nonzero after
  printing the JSON when this array is non-empty.
- `skipped` reports protected or review-only recommendations with a reason.

```json
{
  "total": 1,
  "recommendations": [
    {
      "id": "evidence-id",
      "tool": "vidtrace",
      "category": "orphaned",
      "reason": "source path no longer exists (/path/to/source)",
      "size": 4096
    }
  ],
  "by_category": {"orphaned": 1},
  "reclaimable": 4096,
  "applied": true,
  "dropped": [],
  "reclaimed": 0,
  "failed": [],
  "skipped": [
    {"id": "evidence-id", "reason": "requires review: only explicitly expired or regenerable cache stashes are auto-deletable"}
  ]
}
```

## Integration with ecosystem tools

- **codemap/vecgrep** stashes are regenerable caches and are eligible for
  automatic deletion. Target them with `fcheap cleanup --apply --tool codemap`.
- **vidtrace/cairntrace** stashes are evidence. Missing-source and other heuristic
  recommendations remain review-only unless an explicit TTL expired.
- **glyphrun** artifact packs can be saved and referenced manually; treat them
  as evidence unless the workflow explicitly marks them as regenerable.
- **tinyvault** is not currently an fcheap caller; no special handling is needed.

## See also

- [`sweep`](/cli/sweep) — drop stashes whose TTL has expired
- [`ttl`](/cli/ttl) — set a TTL on a stash
- [`vacuum`](/cli/vacuum) — clean up orphaned DB/index entries
- [Local artifact references](/integrations/local-artifact-references) — hand
  stash metadata to ecosystem tools without moving bytes
