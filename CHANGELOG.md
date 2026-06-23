# Changelog

All notable changes to file.cheap are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Per-release binaries and notes are also on the
[GitHub releases page](https://github.com/abdul-hamid-achik/file.cheap/releases).

## [Unreleased]

_Nothing yet._

## [0.22.0] - 2026-06-23

### Added
- **Studio: live list filter** (`f`) — type to narrow the stash list by name /
  tool / tag; `enter` keeps it, `esc` clears. The cursor and every list action
  operate on the filtered view, with a "N of M" line and a "no match" state.

### Fixed
- **Studio layout** (from an adversarial multi-agent TUI review): the list panel
  no longer drops its bottom border when the list overflows; chip-bearing rows no
  longer wrap at narrow widths; the selected-row highlight matches the panel
  interior; the footer wraps to the terminal width instead of forcing the UI
  wider than the screen; and very short terminals no longer render past the
  bottom.

## [0.21.0] - 2026-06-23

### Added
- **Studio: sort the stash list** — `o` cycles age / name / tool / files / size
  and `O` reverses the direction; the active sort and direction show in the panel
  title (`Stashes · SIZE ▼`) and the sorted column is highlighted in the header.
- **Studio: per-tool color accents** in the list (the TOOL column is colored by
  producing tool), with dimmed size/age columns for quicker scanning.

## [0.20.0] - 2026-06-23

### Added
- **Studio TUI now fills the terminal.** Every view expands to the full height
  between the header and a bottom-pinned footer (previously content sat at the
  top with most of the terminal unused). The stash list is now a proper table —
  aligned `NAME/TOOL/FILES/SIZE/AGE` columns, a NAME column that flexes to the
  available width, right-aligned numbers, chips reserved on the right, a "… N
  more" overflow indicator, and the stash count + total size in the header.
  Detail/search size their preview to the available space.

## [0.19.2] - 2026-06-23

### Security
- Restore with no `--to` now uses a fresh, unique temp directory
  (`os.MkdirTemp`) instead of a predictable `os.TempDir()/<id>` path, so nothing
  can be pre-planted at a known restore destination and repeated restores don't
  merge.
- `Extract` now enforces a total-extraction byte cap (defense-in-depth against a
  decompression bomb that would otherwise fill the disk).

## [0.19.1] - 2026-06-23

Security & robustness fixes from a focused security audit.

### Security
- **Reject path-traversal stash IDs (high).** An unvalidated `stash_id` flowed
  into `filepath.Join(rootDir, id)` then `os.RemoveAll`/restore, so an MCP agent
  calling `fcheap_drop(stash_id: "../../dir", force: true)` had an arbitrary
  directory-deletion primitive. IDs are now validated as a single, non-traversal
  path element across `Drop`/`Restore`/`Info`/`Exists` (and the MCP `diff` tool).
- **Block symlink-escape on restore (medium).** `copyFile` and `Extract` followed
  a symlink pre-planted at a restore destination (no `O_NOFOLLOW`), so a write
  could clobber a file outside the target — they now remove a pre-existing
  symlink before writing. `copyDir` drops absolute/`..`-escaping links on restore
  (matching `Extract`) while still preserving them verbatim on save.

### Changed
- **Atomic writes:** `manifest.Save` uses temp-file + fsync + rename, and
  `Compress` was reordered (rename archive → record manifest → reclaim) so an
  interrupted compress can't leave the manifest/DB diverged from disk.
- **Bounded memory:** the generic and vidtrace detectors cap `SearchableText`
  (4 MiB, via `strings.Builder` instead of O(n²) `+=`), and bundle JSON reads are
  size-capped (32 MiB).

## [0.19.0] - 2026-06-22

Fixes from an exhaustive multi-agent code/docs audit.

### Fixed
- **compress:** `Archive()` discarded final-flush/close errors, so a failed
  flush could be reported as success and `Compress()` would then delete the
  source tree (data loss). Errors are now checked before success is reported.
  `Archive` also no longer crashes on symlinks, and `Extract` recreates them
  safely (rejecting escape links) rather than dropping them.
- **stash/manifest:** `Save()` aborted on a source directory containing a
  dangling symlink; symlinks are now recreated/hashed without dereferencing.
- **analyze:** an embedding-model change bricked all search (incl. keyword);
  search now degrades to BM25 on drift. veclite access is serialized per stash
  root so concurrent MCP calls no longer fail on the file lock, and search/index
  honor context cancellation.
- **config:** `config set`/`config init` no longer bake env/flag overrides into
  `config.yaml`; `config init` requires `--force` to overwrite; `config set`
  validates compression/log_level; `config show --json` emits snake_case.
- **drop:** `drop --json`/`--quiet` without `--force` now returns a non-zero
  error and a structured object instead of a silent exit-0 no-op.
- **version:** `version --json` now emits JSON (was ignored).
- **studio:** `d`/`u` page the preview when it is focused instead of dropping.
- **secrets:** scan no longer silently truncates on an over-long single line.
- **mcp:** the `fcheap://stash/{id}` resource surfaces real read errors instead
  of masking them all as not-found.
- **detect:** vidtrace OCR/transcript text is no longer double/triple-indexed
  (raw `ocr/`/`transcript/` files are skipped when the timeline already yields
  per-frame units), which previously skewed BM25 scores.
- **analyze:** unknown search modes are rejected with a clear error (were
  silently treated as keyword); `DropIndex` no longer creates an empty index DB
  for a never-indexed stash.
- **studio:** very small terminals no longer overflow the layout (inverted
  clamp bounds are tolerated), and a diff launched from the list view is anchored
  to its stash (titled panel + `esc` returns to that stash).

### Changed
- Added a `--no-color` flag (also honors `NO_COLOR`).
- Documentation brought back in sync with the code across CLI/MCP/Studio/AGENTS,
  the in-app Studio help documents the preview pager keys, and `StashQuery`
  truncates on rune boundaries.

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

[Unreleased]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.22.0...HEAD
[0.22.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.2...v0.20.0
[0.19.2]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.1...v0.19.2
[0.19.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.15.1...v0.16.0
