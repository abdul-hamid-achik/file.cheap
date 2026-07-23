# vacuum

Remove orphaned index entries and compact the database.

## Usage

```bash
fcheap vacuum [--json]
```

## What It Does

Over time, a stash directory can disappear without going through
[`drop`](/cli/drop) — for example, deleted manually or by another tool. That
leaves a dangling row in the SQLite metadata index and stale documents in the
veclite search index.

`vacuum`:

1. Scans which stashes physically exist on disk (have a `manifest.json`).
2. Removes metadata-index rows and search-index documents for any stash that no
   longer exists.
3. Compacts the SQLite database (`VACUUM`) to reclaim file space.

It never touches stashes that still exist on disk.

## Output

```
Vacuum complete
  On disk: 12 stash(es)
  Orphans removed: 2
  └─ gone_20260620_101500
  └─ removed_20260619_223000
```

## MCP

Also available to agents as the `fcheap_vacuum` tool. See the
[MCP overview](/mcp/overview).
