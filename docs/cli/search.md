# search

Search across all indexed stashes using per-file BM25 keyword search. Each
readable file is indexed as its own document, so a result names the **exact file**
(or, for vidtrace bundles, the exact frame and timestamp) that matched.

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
| `--limit` | int | `20` | Maximum number of results |
| `--stash` | string | `""` | Limit the search to a single stash ID |
| `--mode` | string | auto | `keyword`, `semantic`, or `hybrid` (default: `hybrid` if an embedder is configured, else `keyword`) |

## Examples

```bash
# Basic search across all indexed stashes
fcheap search "Internal Migrant"

# Cap the number of results
fcheap search "columns not showing" --limit 5

# Scope the search to one stash
fcheap search "checkout" --stash OPG-15061_20260622
```

## How It Works

1. fcheap searches the embedded veclite database (BM25 keyword search by default),
   which holds one document per indexed file (run `fcheap analyze <id>` to index a
   stash).
2. Each result names the stash **and the matching file** (`stash › file`), with a
   relevance score and a snippet centered on the match.

`search` operates purely on your **stashes**. To search a **codebase** (and map a
stashed bug report to the code that owns it), use [`connect`](/cli/connect)
instead — that's where vecgrep comes in.

## Semantic & hybrid search

By default fcheap uses **keyword** (BM25) search. If you configure an embedding
model, it also indexes a vector per document, enabling:

- `--mode semantic` — pure vector similarity (finds related meaning even with no
  shared words).
- `--mode hybrid` — vector + BM25 fused (the default when an embedder is set).

```bash
fcheap config set embedder ollama
fcheap config set embed_model nomic-embed-text   # 768-dim, runs locally via Ollama
fcheap analyze <stash-id>                         # re-index to add vectors
fcheap search "refresh login token" --mode hybrid # finds "renews session credential"
```

Embedders are HTTP-based (`ollama` or `openai`) so the binary stays CGO-free.
Check availability with [`doctor`](/cli/doctor). Without an embedder, `semantic`/
`hybrid` transparently fall back to keyword search.

## Output

```
Search Results (2)

OPG-15061_20260622  ›  logs/app.log
  Score: 1.91 (keyword)
  └─ ...the INTEL_Workers_ITA_International = "Internal Migrant" condition...

OPG-15061_20260622  ›  frames/f2.png @ 12s
  Score: 0.62 (keyword)
  └─ Retry button shown after error
```