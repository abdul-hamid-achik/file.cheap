# search

Search across all indexed stashes using BM25 keyword search.

## Usage

```bash
fcheap search <query> [flags]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `query` | Search query string |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `10` | Maximum number of results |

## Examples

```bash
# Basic search
fcheap search "Internal Migrant"

# Search with more results
fcheap search "columns not showing" --limit 20
```

## How It Works

1. fcheap searches the built-in veclite database (BM25 keyword search)
2. Results include the stash ID, match source, score, and text snippet
3. If vecgrep is configured (`vecgrep_path` in config), also performs semantic search and merges results

## Output

```
Search Results (2)

my_artifacts_20260622_115254
  Source: keyword
  Score: 2.45
  └─ ...the INTEL_Workers_ITA_International = "Internal Migrant" condition...

config_snap_20260622_100000
  Source: keyword
  Score: 0.87
  └─ ...migrant worker configuration...
```