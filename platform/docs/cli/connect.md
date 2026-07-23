# connect

Connect a stash to a codebase: run semantic code search ([vecgrep](https://github.com/abdul-hamid-achik/vecgrep))
over a repository using the stashed artifact's text — e.g. a vidtrace bug
report's OCR and transcript — to rank related `file:line` candidates for
investigation. These matches are leads to verify, not proof of ownership or root
cause.

This is the connective tissue: stash a repro, then point it at the live repo.

## Usage

```bash
fcheap connect <stash-id> <codebase-dir> [flags]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `stash-id` | The stash whose content drives the code search |
| `codebase-dir` | Path to the codebase directory to search |

## Requirements

`connect` requires the separately installed
[vecgrep](https://github.com/abdul-hamid-achik/vecgrep) executable. file.cheap
looks on `PATH` unless `vecgrep_path` is set in its configuration.

```bash
fcheap doctor
```

The target must be a codebase directory that vecgrep can index and search. With
`--index`, file.cheap asks vecgrep to create or refresh its derived repository
index, so the command writes index data inside the target codebase.

Keyword mode does not need an embedding model. Semantic and hybrid modes use
the embedding setup configured by vecgrep; verify that setup independently when
semantic results are empty or unexpectedly weak.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--query` | string | auto | Override the query auto-extracted from the stash |
| `--limit` | int | `10` | Maximum number of code matches |
| `--index` | bool | `false` | Build the vecgrep index for the codebase first (`vecgrep init` + `index`) |
| `--mode` | string | hybrid | vecgrep search mode: `semantic`, `keyword`, or `hybrid` |

## Examples

```bash
# Connect a vidtrace bug bundle to the codebase where the bug lives
fcheap connect OPG-15061 ~/projects/my-app --index

# Narrow with an explicit query and fewer results
fcheap connect OPG-15061 ~/projects/my-app --query "login token refresh" --limit 5
```

## How It Works

1. fcheap derives a query from the stash's searchable text (vidtrace OCR +
   transcript, or generic file content). Override it with `--query`.
2. With `--index`, fcheap runs `vecgrep init` and `vecgrep index .` inside the
   codebase first (idempotent).
3. It runs `vecgrep search` in the codebase and reports ranked code chunks.

### Not indexed (graceful)

If the codebase has no vecgrep index and you didn't pass `--index`, `connect`
does **not** error out. With `--json` it exits `0` and returns
`{"stash_id":…, "matches":[], "index_status":"missing"}`; without `--json` it
prints a hint to re-run with `--index`. A non-zero exit is reserved for real
errors (e.g. vecgrep not installed), so callers can distinguish "not indexed"
from a tool failure.

### JSON shape

Each match is a `SearchResult` with a **clean** `file` path (no `:line`
suffix) and a separate integer `line` field, so callers can build a
`Location{File, StartLine}` without splitting a string:

```json
{
  "stash_id": "OPG-15061_20260622",
  "codebase": "~/projects/my-app",
  "query": "login token refresh failed ...",
  "matches": [
    {"stash_id":"vecgrep","score":0.81,"text":"func refreshToken(t string) ...","file":"auth/login.go","line":3,"source":"vecgrep"}
  ],
  "index_status": "indexed"
}
```

`line` is `0` (omitted) when the match has no known line; `index_status` is
`"indexed"` or `"missing"`.

::: tip Semantic search needs an embedder
vecgrep's `semantic`/`hybrid` modes embed code via its default embedder (ollama +
`nomic-embed-text`). Without that model installed, vecgrep falls back to keyword
matching and `--mode semantic` finds little. Install it with
`ollama pull nomic-embed-text` (the same model fcheap uses for its own semantic
search).
:::

## Output

```
Connect OPG-15061 → ~/projects/my-app

  Query: login token refresh failed with 401 unauthorized it logs me out...

Candidate code (3)
  auth/login.go:3: score 0.81
  └─ func refreshToken(t string) (string, error) { ... }
  auth/session.go:42: score 0.55
  └─ func (s *Session) invalidate() { ... }
```

## MCP

The same capability is exposed to agents as the `fcheap_connect` tool. See the
[MCP overview](/mcp/overview).
