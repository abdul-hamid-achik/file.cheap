# CLAUDE.md

Guidelines for Claude Code working on the file.cheap codebase.

## What This Project Is

file.cheap is a local-first CLI tool + MCP server that saves, restores,
compresses, analyzes, and diffs files and folders for agent workflows. The
binary is called `fcheap`; its source of truth is local. The repository also
contains an isolated public Next.js site and a private artifact service under
`platform/`.

## Architecture Quick Reference

- `internal/stash/` -- core domain (Save, Restore, Drop, List, Info)
- `internal/manifest/` -- snapshot metadata + provenance
- `internal/compress/` -- tar+zstd archiving
- `internal/detect/` -- bundle type detection (vidtrace, generic)
- `internal/analyze/` -- BM25 keyword search (veclite) + vecgrep subprocess
- `internal/diff/` -- stash-to-directory comparison
- `internal/db/` -- SQLite metadata (modernc.org/sqlite, CGO-free)
- `internal/mcp/` -- MCP server: 15 tools (incl. fcheap_docs) + resources (`fcheap://stashes`, `fcheap://stash/{id}`) + prompts (`investigate_stash`, `find_across_stashes`) (modelcontextprotocol/go-sdk)
- `internal/studio/` -- Bubbletea v2 TUI
- `internal/fcheap/cli/` -- Cobra CLI commands
- `internal/fcheap/config/` -- YAML config + env overrides
- `internal/fcheap/output/` -- printer, tables, progress
- `internal/fcheap/version/` -- build-time version injection
- `platform/` -- the single Next.js/Vercel public site
- `platform/docs/` -- VitePress source, staged into the Next build and embedded in the Go binary

## Key Patterns

### MCP Tools (modelcontextprotocol/go-sdk v1.2.0)

The `jsonschema` tag is a plain description string. Do NOT use `description=` prefix or `required` keyword (required is handled by omitting `omitempty` on the `json` tag):

```go
type myInput struct {
    Path string `json:"path" jsonschema:"Absolute path to the file or directory"`
    Name string `json:"name,omitempty" jsonschema:"Display name for the stash"`
}
```

### CLI Commands

Each command is in `internal/fcheap/cli/<name>.go`, registered in `root.go` `init()`. The `docs` command provides `serve`, `build`, `preview`, `list`, `show`, and `open` subcommands for the VitePress docs site.

### Build & Test

```bash
task build          # build binary
task test           # run tests
task vet            # go vet
task lint           # golangci-lint
task check          # full check (fmt, tidy, vet, lint, test, build)
task e2e            # run glyphrun e2e specs
```

## Documentation

This project uses **Obsidian CLI** for note-taking and knowledge management. The Obsidian vault for this project is at `~/notes/projects/file.cheap`.

When you need to document something, add a note, or capture a decision:
1. Use the Obsidian CLI to create or edit notes in the vault
2. Notes should follow the existing vault structure (folders by topic)
3. The vault is the single source of truth for project knowledge

The `platform/docs/` directory is the public VitePress source. It is not a
separate deployment: the `platform/` build stages it under `public/_docs` and
serves its historical routes from the existing `file-cheap` Vercel project.

- `~/notes/projects/file.cheap` (Obsidian vault) -- internal notes, decisions, research
- `platform/docs/` (VitePress source) -- public-facing documentation at file.cheap

## What NOT to Do

- Don't add HTTP server/API code to the Go core
- Don't add authentication or billing to the Go core
- Don't import cloud SDKs (S3, GCS, etc.) into the Go core
- Don't add telemetry, metrics, or tracing to the Go core
- Don't bundle external binaries
- Don't use `os/exec` outside of `internal/analyze/` (for vecgrep subprocess) and `internal/fcheap/cli/docs.go` (for VitePress dev/build)
- Don't add database dependencies beyond embedded SQLite
- Don't create documentation files (*.md) in the repo root unless explicitly requested -- use the Obsidian vault instead
