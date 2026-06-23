# Changelog

All notable changes to file.cheap are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Per-release binaries and notes are also on the
[GitHub releases page](https://github.com/abdul-hamid-achik/file.cheap/releases).

## [Unreleased]

_Nothing yet._

## [0.18.0] - 2026-06-22

### Added
- **MCP resources** — read stash data by URI without spending a tool call:
  - `fcheap://stashes` — JSON index of every stash.
  - `fcheap://stash/{id}` — a single stash's full manifest (resource template).
- **MCP prompts** — one-shot agent workflows:
  - `investigate_stash` — manifest → analyze/search → connect → summarize.
  - `find_across_stashes` — cross-stash search and synthesis.

### Changed
- `fcheap_list` and the `fcheap://stashes` resource now share a single
  `stashSummary` helper (removing a duplicated summary path and the
  `manifest` keep-alive hack in `server.go`).

### Fixed
- Unchecked `gzReader.Close()` on the decompress path (flagged by
  golangci-lint v2's `errcheck`).

### Internal
- Pinned `task lint` to golangci-lint **v2.7.2** so local lint matches CI and
  can't drift green-locally / red-in-CI again.

## [0.17.0] - 2026-06-22

### Added
- **Hybrid & semantic search** (veclite) — opt-in embedder (`ollama`/`openai`,
  CGO-free) adds a vector index alongside BM25. `search --mode keyword|semantic|hybrid`
  (auto-hybrid when an embedder is configured), embedding-profile drift detection,
  and graceful BM25 fallback.
- **`fcheap connect`** — run semantic code search (vecgrep) over a codebase using a
  stashed artifact's text to surface the `file:line` candidates most likely
  responsible for a bug. Exposed as the `fcheap_connect` MCP tool.
- **Secret detection** — a save-time scan (`internal/secrets`) flags likely
  credentials, surfaced in the CLI, MCP, and Studio (a "secrets" chip/warning).
- **`fcheap vacuum`** — remove orphaned metadata/search-index rows and compact the
  database. Exposed as the `fcheap_vacuum` MCP tool.
- **`fcheap version`** and a `doctor` embedder check.
- **SQLite metadata** via sqlc (`internal/db`), wired into stash operations with
  graceful degradation when the DB is unavailable.
- **Auto-compress on save** when a stash exceeds the configured threshold.
- **Studio TUI redesign** (charm.land bubbletea/lipgloss/bubbles v2): list, detail,
  search, diff, timeline, status, and help views; themed colors and chips; spinner
  and progress-bar animations; responsive layout; and a search-mode cycle.
- MCP server expanded to **11 tools**, including `fcheap_docs` for reading the docs.

### Changed
- The logger now writes text to **stderr** (it previously emitted JSON to stdout,
  which could corrupt `--json` output).
- Config `embedder` / `embed_model` / `ollama_url` are now wired end-to-end.

### Removed
- The dead `parallel` / `FCHEAP_JOBS` config knob.

## [0.16.0] - 2026-06-22

### Changed
- **BREAKING:** Rewrote the project as a local-first **stash tool for agent
  workflows** — save, restore, drop, list, info, compress, diff, and analyze files
  and folders, with an MCP server, a Studio TUI, and VitePress docs. The CLI binary
  is `fcheap`.

## Earlier releases

Versions **0.1.0 – 0.15.1** (January–February 2026) predate the stash rewrite.
See the [GitHub releases page](https://github.com/abdul-hamid-achik/file.cheap/releases)
for their notes and binaries.

[Unreleased]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.18.0...HEAD
[0.18.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.15.1...v0.16.0
