# AGENTS.md

Guidelines for AI agents working on the file.cheap codebase.

## Architecture

file.cheap's shipped product is a local-first CLI tool + MCP server for saving,
restoring, compressing, and analyzing files and folders for agent workflows. The
Go core stores files locally on the user's machine and has no dependency on the
optional hosted platform.

The repository also contains `platform/`, the unified Next.js public website,
VitePress documentation source, and recovery-protocol laboratory. The existing
Vercel project `file-cheap` builds only this directory. The recovery lab and
`/api/v1` are disabled by default in hosted environments, and the multi-customer
remote vault is not shipped. The platform rules below apply only inside that
directory.

### Key Layers

1. **Stash** (`internal/stash/`) -- the core domain. The `Manager` handles Save, Restore (with hash verification), Drop, List, Info, and Compress operations on file/folder snapshots. Save also scans content for likely secrets (`internal/secrets/`) and records findings in the manifest. Zero coupling to infrastructure.

2. **Manifest** (`internal/manifest/`) -- snapshot metadata: ID, name, tags, tool, source path, file count, size, hashes, bundle type. Serialized as `manifest.json` alongside each stash.

3. **Compress** (`internal/compress/`) -- tar+zstd and tar+gzip archiving. Streaming archive/extract for space-efficient storage.

4. **Detect** (`internal/detect/`) -- bundle type detection. Recognizes vidtrace bundles (metadata.json + timeline.json) and generic file trees. Extracts searchable text per type.

5. **Analyze** (`internal/analyze/`) -- per-file search via the embedded **veclite** vector database (one document per file, tagged with stash ID + relative path). BM25 keyword search by default; when an embedder (`ollama`/`openai`, HTTP so CGO-free) is configured via `EmbedderSettings`/`.WithEmbedder()`, documents also carry a vector in a `files_vec` collection, enabling `search --mode semantic|hybrid` with graceful BM25 fallback. Optional vecgrep subprocess for semantic code search. Index lives at `<stash-dir>/fcheap.veclite`.

6. **Diff** (`internal/diff/`) -- compares a stash against a target directory. Reports files only in stash, only in target, and changed files.

7. **DB** (`internal/db/`) -- SQLite (via modernc.org/sqlite, CGO-free) for queryable metadata. Schema (`schema.sql`) and queries (`queries.sql`) drive **sqlc**-generated code in `internal/db/gen/`; a thin `Store` wraps it. The manifest.json files remain the source of truth — the DB is a write-through index that self-heals on `List`. Regenerate with `task sqlc-gen`. Index lives at `<stash-dir>/fcheap.db`.

8. **MCP Server** (`internal/mcp/`) -- exposes stash operations via `modelcontextprotocol/go-sdk` across all three MCP surfaces. **Tools** (`server.go`) use typed input structs with `json` + `jsonschema` tags for auto-schema generation; each validates input, calls the stash manager, returns JSON (incl. `fcheap_docs` for reading documentation). **Resources** and **prompts** (`resources.go`) expose stash data by URI (`fcheap://stashes`, `fcheap://stash/{id}`) and one-shot agent workflows (`investigate_stash`, `find_across_stashes`).

9. **CLI** (`internal/fcheap/cli/`) -- Cobra commands. Each command file handles args/flags, calls stash manager, prints output via the printer. Includes `docs` command for serving, building, and reading the VitePress docs.

10. **Studio TUI** (`internal/studio/`) -- Bubbletea v2 terminal interface for browsing stashes, viewing manifests, and triggering operations.

11. **Docs** (`platform/docs/`) -- VitePress source embedded in the `fcheap`
    binary and staged into `platform/public/_docs` during the same Next.js build.
    Internal rewrites preserve the historical
    `file.cheap/{guide,cli,mcp,...}` URLs. The `fcheap docs` CLI command serves,
    builds, lists, and reads doc pages.

12. **Public Platform** (`platform/`) -- isolated Next.js website plus a gated,
    single-workspace recovery laboratory. It is the sole public deployment and
    remains a runtime dependency boundary. The only Go import allowed from this
    tree is the static `platform/docs` content package used by CLI and MCP
    embedding; the Go core must never import the Next.js application or cloud
    adapters.

## Code Style

- Go 1.25+, `CGO_ENABLED=0`
- No generics unless the stdlib pattern demands it
- Errors: wrap with `fmt.Errorf("context: %w", err)`, use `apperror` types at boundaries
- Logging: `internal/logger` (slog-based). Use `slog.Debug`/`slog.Info`/`slog.Error`
- Tests: `testing` + `testify/assert`. Test files next to source
- Lint: `golangci-lint` with `errcheck` enabled. All deferred `.Close()` calls on read-only files use `//nolint:errcheck` comment

## Conventions

### Adding a New CLI Command

1. Create `internal/fcheap/cli/<name>.go` with a cobra command
2. Register in `internal/fcheap/cli/root.go` `init()`
3. If it needs MCP exposure, add a tool in `internal/mcp/server.go`
4. Add e2e spec in `e2e/flows/cli_<name>.yml`
5. If it needs a docs page, add `platform/docs/cli/<name>.md` and update `platform/docs/.vitepress/config.ts` nav

### MCP Tools

Tools use the official Go SDK pattern:

```go
type myInput struct {
    Path string `json:"path" jsonschema:"Absolute path to the file or directory"`
}

mcp.AddTool(srv, &mcp.Tool{
    Name:        "fcheap_my_tool",
    Description: "...",
    Annotations: &mcp.ToolAnnotations{
        DestructiveHint: &f,
        OpenWorldHint:   &t,
        IdempotentHint:  true,
    },
}, func(ctx context.Context, req *mcp.CallToolRequest, in myInput) (*mcp.CallToolResult, any, error) {
    // validate, process, return
})
```

Note: `DestructiveHint` and `OpenWorldHint` are `*bool` (use `&f`/`&t` helpers). `IdempotentHint` is plain `bool`.

### CLI Commands

Each command follows the pattern:

```go
var myCmd = &cobra.Command{
    Use:   "my <args...>",
    Short: "...",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        mgr, err := stash.NewManager(cfg.StashDir)
        // call mgr, print results via printer
    },
}
```

Register in `root.go` `init()`.

### Studio TUI

Built with `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`. Uses `tea.NewView()` for View() return. Interactive guard checks `term.IsTerminal()` on stdin/stdout.

### E2E Tests

Built with glyphrun. Specs live in `e2e/flows/`. Each spec builds the binary as a precondition, runs CLI commands in a PTY, and verifies outcomes via screen content and exit codes.

## Public Platform Conventions

These conventions apply only to `platform/`. They narrow the Go core's
prohibition on HTTP servers and cloud SDKs so the isolated website can contain a
Next.js control plane and Vercel Blob adapter. Never import the application or
its dependencies from the local-first Go core.

### Scope and release state

- The Next.js root page is the public product website. The existing
  `file-cheap` Vercel project deploys `platform/`; code readiness does not
  authorize a production domain cutover.
- The recovery lab and stateful `/api/v1` routes are a single-workspace
  experiment. Keep them disabled on public deployments. `/api/v1/health` may
  report that the public site is healthy without initializing storage.
- A static bearer token is acceptable only for local development or a
  controlled preview protected by Vercel access controls.
- The lab is not a public remote vault. Do not send it customer data or use it
  as the integration boundary for Chalupa, Cairntrace, or Glyphrun.

### Technology and boundaries

- Use Bun for installs and scripts. Do not add npm or Yarn lockfiles.
- Use Next.js App Router, strict TypeScript, and Route Handlers under
  `platform/src/app/api/v1/`.
- Keep `/` independent of storage credentials and recovery configuration.
- Build VitePress locally into `platform/public/_docs` before every Next.js
  build. Never restore `FILECHEAP_DOCS_ORIGIN` or an external docs rewrite.
- Keep `/_docs/*` as an internal build namespace. Public links and canonical
  metadata must use the historical clean routes.
- Route Handlers authenticate, validate, call a feature service, and translate
  errors. Business rules belong in `platform/src/features/`.
- Provider SDKs belong behind ports in `platform/src/platform/`. Only the
  Vercel Blob adapter may import `@vercel/blob`.
- Local adapter data is disposable and lives under `platform/.data/`.
- Archive objects are immutable and SHA-256 addressed. Never overwrite one
  silently or bind one stash ID to two hashes.
- Large archive bytes require direct signed transfers in a future production
  design; do not proxy them through a Vercel Function.
- API errors use `application/problem+json` and the RFC 9457 shape.

### Recovery-lab constraints

- `PLATFORM_RECOVERY_LAB_ENABLED` must be false or absent on the public
  deployment. An explicit `true` is only for a controlled, access-protected
  preview.
- Do not add auth providers, payments, email, teams, continuous sync, public
  sharing, background deletion, or telemetry to the lab.
- Blob is not a production multi-tenant catalog. Add a transactional database
  before any external multi-customer beta.
- Never infer remote safety from `HEAD` or ETag alone. The prototype never
  evicts local content. A future encrypted client needs a complete
  hydrate-and-hash check and recovery-key export before eviction is considered.

### Platform verification

Run from `platform/`:

```sh
bun install --cwd docs --frozen-lockfile
bun run check
bun run audit:production
```

The check performs linting, strict type checking, unit and contract tests, a
production build, and an isolated recovery E2E. The audit must report no known
production dependency vulnerabilities. Before a domain cutover, follow the root
[`DEPLOYMENT.md`](DEPLOYMENT.md); preview, domain assignment, and rollback are
separate release gates.

## Storage Layout

```
~/.local/share/fcheap/           (XDG_DATA_HOME or ~/.local/share)
├── <stash-id>/
│   ├── manifest.json            # metadata, provenance, tags
│   └── content/                 # extracted file tree
│       (or content.tar.zst / content.tar.gz / content.tar when compressed)
├── fcheap.db                    # SQLite metadata index (sqlc)
└── fcheap.veclite               # veclite search index (BM25 + optional vectors)
```

## External Dependencies

- **vecgrep** -- optional, for semantic search via subprocess. Detected at runtime, `fcheap doctor` reports status.
- **zstd** -- built-in via `github.com/klauspost/compress/zstd` (no external binary needed).

Never bundle external binaries. Detect at runtime, show clear errors with `fcheap doctor` instructions.

## What NOT to Do

These restrictions apply to the Go local-first core. The public platform has
only the narrow exceptions documented above.

- Don't add HTTP server/API code to the Go core
- Don't add authentication or billing to the Go core
- Don't import cloud SDKs (S3, GCS, etc.) into the Go core
- Don't add telemetry, metrics, or tracing to the Go core
- Don't bundle external binaries
- Don't use `os/exec` outside of `internal/analyze/` (for vecgrep subprocess) and `internal/fcheap/cli/docs.go` (for VitePress dev/build)

## Documentation

This project uses **Obsidian CLI** for note-taking and knowledge management. The Obsidian vault for this project is at `~/notes/projects/file.cheap`.

When you need to document something, add a note, or capture a decision:

1. Use the Obsidian CLI to create or edit notes in the vault
2. Notes should follow the existing vault structure (folders by topic)
3. The vault is the single source of truth for project knowledge -- architecture decisions, debugging notes, feature plans, etc.

The `platform/docs/` directory is specifically for the public **VitePress
documentation source**. It contains user-facing Markdown, is embedded into the
CLI/MCP package, and is built as part of the one public platform deployment. Do
not confuse it with the internal vault:

- `~/notes/projects/file.cheap` (Obsidian vault) -- internal notes, decisions, research
- `platform/docs/` (VitePress source) -- public-facing documentation at
  `file.cheap/guide`, `file.cheap/cli`, and the other historical docs routes

Architecture decisions live only in the Obsidian vault, not under `platform/`.
The current decisions are:

- `projects/file.cheap/ADR-001-blob-first-recovery-prototype.md`
- `projects/file.cheap/ADR-002-stripe-billing-boundary.md`
- `projects/file.cheap/ADR-003-public-site-and-docs-zones.md`
