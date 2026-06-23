# Studio Overview

The Studio is fcheap's terminal user interface (TUI) for browsing and managing stashes interactively.

## Launching

```bash
fcheap studio
```

The Studio requires an interactive terminal. It checks `term.IsTerminal()` on both stdin and stdout before launching. If run in a non-interactive context (piped input/output), it exits with an error message suggesting CLI commands instead.

## Views

### List

The default view: a themed table of stashes (ID, tool, file count, size, relative
age) with colored chips — `zst`/`gz` for compressed stashes and a `⚠ secrets` chip
when a stash contains likely credentials. Newest first. `j`/`k` to move, `Enter` to
open detail.

### Detail

Provenance (ID, name, source, tool, created, bundle type), tags as chips,
size/hash, compression status, and a secrets warning when present. A scrollable
**file tree** sits beside a live **preview** of the selected file's content
(`Tab` cycles focus). Compressed stashes show a "restore to view" placeholder.

### Search

Press `/` to focus a working query input, type, and `Enter` to run a per-file
search across all indexed stashes. Results show `stash › file` with a
relevance-colored score (green → cyan → dim), and the preview pane shows the hit.
`Tab` cycles query ↔ results ↔ preview. Press `m` (from the results/preview pane)
to cycle the search **mode** — `auto` → `keyword` → `semantic` → `hybrid` — and
re-run; the query panel shows the requested mode and the results panel shows the
mode actually used (semantic/hybrid require a configured embedder).

### Timeline (vidtrace bundles)

Press `t` on a vidtrace bundle to see its evidence timeline — each entry's
timestamp, frame, OCR text, and transcript — in a scrollable, colored panel.

### Diff

Press `x` to diff a stash against a directory. The prompt is pre-filled with the
stash's source path (just press `Enter`), or type any path. Results are colored:
green `+` (only in stash), red `-` (only in target), yellow `~` (changed), plus
an unchanged count — comparison is by content hash.

### Status & Help

`s` shows index health at a glance: stash directory, count, total size, how many
stashes are indexed/compressed, whether the SQLite metadata index and veclite
search index exist, and vecgrep availability. `?` shows the keybinding reference.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` (`↓`/`↑`) | Move cursor / scroll |
| `Enter` / `l` | Open detail |
| `Esc` / `h` | Back |
| `Tab` | Cycle pane focus |
| `/` | Search |
| `r` | Restore to a temp dir (with hash verification) |
| `c` | Compress (zstd) |
| `a` | Analyze / index for search |
| `x` | Diff against a directory |
| `t` | vidtrace timeline (bundles only) |
| `d` | Drop (with `y/n` confirm) |
| `s` | Status |
| `g` | Refresh list |
| `?` | Help |
| `q` | Quit · `Ctrl+C` force quit |

Async operations (loading, search, restore, drop, compress) show an animated
spinner while in flight. Indexing (`a`) streams per-file progress into an
animated progress bar (`indexing 42/805`). Destructive `d` requires a `y/n`
confirm in the footer.

## Tech Stack

- [Bubbletea v2](https://charm.land) -- TUI framework (`charm.land/bubbletea/v2`)
- [Lipgloss v2](https://charm.land) -- styling (`charm.land/lipgloss/v2`)
- [Bubbles v2](https://charm.land) -- `textinput`, `viewport`, `spinner`, `progress` (`charm.land/bubbles/v2`)

Layout is responsive: side-by-side panels at ≥96 columns, stacked below.