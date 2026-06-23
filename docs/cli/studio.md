# studio

Open the Studio TUI for browsing stashes interactively.

## Usage

```bash
fcheap studio
```

## What It Shows

The Studio TUI provides a terminal interface for browsing and managing stashes:

- **List**: themed table with compression + `⚠ secrets` chips
- **Detail**: provenance, file tree, and a live file preview
- **Search**: a working query input with relevance-colored, per-file results
- **Timeline**: the vidtrace evidence timeline (frame → OCR → transcript)
- **Status / Help**: system info and the keybinding reference

See the [Studio overview](/studio/overview) for the full keymap. Highlights:

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate / scroll |
| `Enter` / `l` | Open stash detail |
| `/` | Search stash content |
| `r` `c` `a` | Restore · compress · index |
| `x` | Diff against a directory |
| `t` | vidtrace timeline (bundles) |
| `d` | Drop (with `y/n` confirm) |
| `q` / `Esc` | Back / quit · `?` help |

## Requirements

The Studio requires an interactive terminal. It will not work in non-interactive mode (piped input/output). In that case, use the CLI commands instead.

## Technical Details

Built with [Bubbletea v2](https://github.com/charmbracelet/bubbletea) and [Lipgloss v2](https://github.com/charmbracelet/lipgloss) from the Charm ecosystem. The TUI uses `tea.NewView()` for rendering and checks `term.IsTerminal()` before launching.