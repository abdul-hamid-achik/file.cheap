# ecosystem-status

Show a read-only dashboard of stashes grouped by the tool that produced them,
including an estimate of potentially reclaimable storage.

## Usage

```bash
fcheap ecosystem-status [--json]
```

The command has no command-specific flags. It honors the
[global flags](/cli/#global-flags).

## What it reports

For each `tool` value, the table shows:

| Field | Meaning |
|---|---|
| `COUNT` | Number of stashes from the tool |
| `STORED` | Estimated bytes currently occupied by content, accounting for compressed archives |
| `OLDEST` | Age in whole days of the oldest stash |
| `EXPIRED` | Stashes whose TTL has elapsed |
| `ORPHANED` | Stashes whose recorded source path no longer exists |

Stashes without a tool are grouped under `-`. Unlike the default `list`
command, the dashboard includes expired stashes so its totals describe the
whole local vault.

The summary distinguishes:

- **logical size**: the original uncompressed payload sizes recorded by
  manifests;
- **stored content estimate**: current payload bytes after compression;
- **recommended cleanup savings**: the size of candidates identified by the
  smart cleanup analysis with a 30-day stale threshold.

## Safety

`ecosystem-status` never compresses or deletes a stash. The cleanup value is an
estimate, not authorization to remove everything it counts.

Review candidates with:

```bash
fcheap cleanup --smart
fcheap sweep
```

Both are also previews by default. Apply deletion only after reviewing the plan
and confirming that unique evidence is not being treated as a regenerable cache.

## Example

```text
Ecosystem Status

TOOL       COUNT  STORED  OLDEST  EXPIRED  ORPHANED
codemap        8  42 MB   18d            3         1
vidtrace       3  1.2 GB  41d            0         0
-              2  16 KB   2d             0         0

Total: 13 stashes, 1.8 GB logical
Stored content estimate: 1.2 GB
Recommended cleanup savings: 42 MB
```

Values depend on the current vault; this example illustrates the fields rather
than an exact formatting contract.

## JSON output

Use JSON for automation:

```bash
fcheap ecosystem-status --json
```

The top level contains `tools` and `overall`. Per-tool values include `count`,
`logical_size`, `stored_size`, `oldest_age_seconds`, `expired`, and `orphaned`.
The overall object includes `total_stashes`, logical and stored sizes,
human-readable usage strings, `reclaimable_size`, and the underlying
`cleanup_result`.

`total_size` remains as a deprecated alias for `logical_size`; new integrations
should use the explicit field names.

## See also

- [`cleanup`](/cli/cleanup) — inspect heuristic and category-based candidates
- [`sweep`](/cli/sweep) — preview or apply expired-stash removal
- [`compress`](/cli/compress) — reduce stored bytes without deleting the stash
- [`vacuum`](/cli/vacuum) — repair and compact derived indexes
