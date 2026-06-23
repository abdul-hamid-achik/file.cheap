# connect

Connect a stash to a codebase: run semantic code search ([vecgrep](https://github.com/abdul-hamid-achik/vecgrep))
over a repository using the stashed artifact's text — e.g. a vidtrace bug
report's OCR and transcript — to surface the `file:line` candidates most likely
responsible for the bug.

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
fcheap connect OPG-15061 ~/projects/graphite --index

# Narrow with an explicit query and fewer results
fcheap connect OPG-15061 ~/projects/graphite --query "login token refresh" --limit 5
```

## How It Works

1. fcheap derives a query from the stash's searchable text (vidtrace OCR +
   transcript, or generic file content). Override it with `--query`.
2. With `--index`, fcheap runs `vecgrep init` and `vecgrep index .` inside the
   codebase first (idempotent).
3. It runs `vecgrep search` in the codebase and reports ranked code chunks.

Requires `vecgrep` on `PATH` (or `vecgrep_path` in config). Check with
[`doctor`](/cli/doctor).

::: tip Semantic search needs an embedder
vecgrep's `semantic`/`hybrid` modes embed code via its default embedder (ollama +
`nomic-embed-text`). Without that model installed, vecgrep falls back to keyword
matching and `--mode semantic` finds little. Install it with
`ollama pull nomic-embed-text` (the same model fcheap uses for its own semantic
search).
:::

## Output

```
Connect OPG-15061 → ~/projects/graphite

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
