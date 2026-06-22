# studio

Open the Studio TUI for browsing stashes interactively.

## Usage

```bash
fcheap studio
```

## What It Shows

The Studio TUI provides a terminal interface for browsing and managing stashes:

- **List view**: All stashes with name, tool, tags, file count, size
- **Detail view**: Manifest metadata, file tree, provenance
- **Help**: Keybindings reference

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Open stash detail |
| `m` | Toggle metadata view |
| `o` | Open file externally |
| `r` | Reveal file in file manager |
| `c` | Copy path to clipboard |
| `q` / `Esc` | Quit |
| `?` | Show help |

## Requirements

The Studio requires an interactive terminal. It will not work in non-interactive mode (piped input/output). In that case, use the CLI commands instead.

## Technical Details

Built with [Bubbletea v2](https://github.com/charmbracelet/bubbletea) and [Lipgloss v2](https://github.com/charmbracelet/lipgloss) from the Charm ecosystem. The TUI uses `tea.NewView()` for rendering and checks `term.IsTerminal()` before launching.