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
fcheap analyze my_artifacts_20260622_115254 --query "checkout retry"
```

## What Happens

1. Detects the bundle type (vidtrace, generic)
2. Extracts searchable text from text files (OCR, transcripts, source code, etc.)
3. Indexes the content using veclite (BM25 plus vectors when an embedder is configured)
4. If `--query` is provided, searches within the stash and prints results

## Remote embedding safety

BM25 indexing is local and does not require an embedder. Ollama embedding runs
at localhost by default. OpenAI and non-loopback Ollama endpoints receive
document text, so fcheap checks the save-time secret flag before remote
indexing.

If the scanner found potential secrets, indexing stops before sending content
unless `allow_remote_secrets: true` is explicitly configured. Review the stash or
use a loopback Ollama endpoint instead of bypassing the guard by default. See
[`config`](/cli/config#remote-embedding-safety).

If you also pass `--query`, semantic/hybrid search sends that query to the
configured embedder. Query text is not covered by the save-time secret guard.

## Bundle Detection

fcheap automatically detects bundle types:

- **vidtrace**: directories containing `metadata.json` + `timeline.json`. Extracts OCR text and transcript segments.
- **generic**: any other directory. Indexes all text-readable files.

## Output

```
Indexed stash: my_artifacts_20260622_115254
  Files indexed: 803
  Bundle: vidtrace
```

With `--query` (each hit names the matching file and its score):

```
Search Results (3)

logs/app.log: score 2.45
  ...checkout retry condition is not working...
```
