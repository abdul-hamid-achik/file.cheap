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
| `--apply` | bool | `false` | Actually drop stashes (default: dry-run) |
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
| `--projects-dir` | string | `~/projects` | Path to ~/projects for orphan detection |
| `--notes-dir` | string | `~/notes/projects` | Path to ~/notes/projects for orphan detection |

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
| **drop** | >= 60 | Dropped |
| **review** | 30-59 | Not dropped (needs manual review) |
| **keep** | < 30 | Not dropped |

## Smart mode (`--smart`)

Categorizes every stash into exactly one cleanup category based on why it might be droppable:

| Category | Description |
|----------|-------------|
| `expired` | TTL has elapsed |
| `orphaned` | Source path (or project directory) no longer exists |
| `superseded` | A newer stash exists for the same tool + source path |
| `duplicate` | Same content hash as a newer stash |
| `branch-gone` | A `branch:` tag references a deleted git branch |
| `stale` | Older than `--stale-days` (uses `created_at` as proxy) |
| `keep` | No cleanup reason found |

Priority order ensures each stash gets only its first matching category (expired beats orphaned beats superseded, etc.) — no double counting.

With `--apply`, all non-keep stashes are dropped (respecting `--categories` filter and `--keep-tag`).

### Keep-tag protection

In both modes, stashes bearing the keep-tag (default: `keep`) are never dropped:

- **Scoring mode**: the keep-tag is a **hard floor** — the stash always gets a `keep` verdict regardless of its score. Even a cache-tool stash with expired TTL and source-gone will not be dropped if it has the keep tag.
- **Smart mode**: stashes with the keep-tag are skipped during `--apply` even if they're categorized as expired/orphaned/etc.

This is a safety net for pinning important stashes that should survive any cleanup.

## Examples

```bash
# Scoring mode: dry-run, see all stashes scored
fcheap cleanup

# Only show stashes that are safe to drop
fcheap cleanup --drop-only

# Actually drop high-confidence candidates
fcheap cleanup --apply

# Analyze only codemap snapshots
fcheap cleanup --tool codemap

# Smart mode: categorize all stashes
fcheap cleanup --smart

# Smart mode: only show expired and orphaned stashes
fcheap cleanup --smart --categories expired,orphaned

# Smart mode: drop expired and duplicate stashes
fcheap cleanup --smart --apply --categories expired,duplicate

# Smart mode: find stashes older than 30 days
fcheap cleanup --smart --stale-days 30
```

## Safety

- Stashes with the `keep` tag (configurable via `--keep-tag`) are **never dropped** in either mode. In scoring mode, the keep tag is a hard floor that forces a `keep` verdict regardless of the computed score. In smart mode, stashes with the keep tag are skipped during `--apply` even if categorized as expired/orphaned/etc.
- In scoring mode, only stashes with verdict **drop** are affected by `--apply`. **review** and **keep** stashes are never dropped.
- In smart mode, `--apply` drops all non-keep stashes (excluding keep-tagged ones). Use `--categories` to limit which categories are dropped.
- Both modes are dry-run by default — no stashes are dropped without `--apply`.

## Integration with ecosystem tools

- **codemap/vecgrep** stashes are the strongest cleanup candidates (scoring: +25 cache tool, often +35 source gone; smart: often orphaned or superseded). Tag them for targeted cleanup: `fcheap cleanup --apply --tool codemap`.
- **vidtrace/cairntrace** stashes are protected (scoring: -30 evidence tool; smart: still categorized but should be reviewed carefully).
- **glyphrun/tinyvault** — not currently fcheap callers; no special handling needed.

## See also

- [`sweep`](/cli/sweep) — drop stashes whose TTL has expired
- [`ttl`](/cli/ttl) — set a TTL on a stash
- [`vacuum`](/cli/vacuum) — clean up orphaned DB/index entries