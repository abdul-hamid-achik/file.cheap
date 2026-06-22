# analyze

Index a stash for search and optionally search within it.

## Usage

```bash
fcheap analyze <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--query` | string | `""` | Optional search query within the stash |

## Examples

```bash
# Index a stash for search
fcheap analyze my_artifacts_20260622_115254

# Index and search in one step
fcheap analyze my_artifacts_20260622_115254 --query "Internal Migrant"
```

## What Happens

1. Detects the bundle type (vidtrace, generic)
2. Extracts searchable text from text files (OCR, transcripts, source code, etc.)
3. Indexes the content using veclite (BM25 keyword search)
4. If `--query` is provided, searches within the stash and prints results

## Bundle Detection

fcheap automatically detects bundle types:

- **vidtrace**: directories containing `metadata.json` + `timeline.json`. Extracts OCR text and transcript segments.
- **generic**: any other directory. Indexes all text-readable files.

## Output

```
Indexed stash: my_artifacts_20260622_115254
  Bundle: vidtrace
  Searchable files: 803
```

With `--query`:

```
Search Results (3)

my_artifacts_20260622_115254
  Source: keyword
  Score: 2.45
  └─ ...Internal Migrant conditions not showing...
```