# ttl

Set or update the time-to-live (TTL) for a stash. A stash with a TTL will auto-expire after the given duration from its creation time. Expired stashes are hidden from `list` by default and can be dropped with `fcheap sweep`.

## Usage

```bash
fcheap ttl <stash-id> <duration> [flags]
```

Pass an empty string `""` as the duration to clear the TTL (make the stash permanent).

## Duration formats

| Format | Example | Meaning |
|--------|---------|---------|
| Go duration | `24h`, `90m` | Hours/minutes from creation |
| Day shorthand | `7d`, `30d` | N days from creation |
| Week shorthand | `2w` | N weeks from creation |
| Date | `2026-12-31` | Expire on this date |

## Examples

```bash
# Set a 7-day TTL on an existing stash
fcheap ttl my_artifacts_20260622_115254 7d

# Set a 30-day TTL
fcheap ttl my_artifacts_20260622_115254 30d

# Expire on a specific date
fcheap ttl my_artifacts_20260622_115254 2026-12-31

# Clear the TTL (make permanent)
fcheap ttl my_artifacts_20260622_115254 ""
```

## Output

```
✓ Set TTL on stash: my_artifacts_20260622_115254
  Expires: 2026-06-29T11:52:54Z
```

## See also

- [`save --ttl`](/cli/save) — set a TTL at save time
- [`sweep`](/cli/sweep) — drop stashes whose TTL has expired
- [`list`](/cli/list) — expired stashes are hidden by default; use `--include-expired` to see them