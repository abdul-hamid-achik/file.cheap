# Studio Overview

The Studio is fcheap's terminal user interface (TUI) for browsing and managing stashes interactively.

## Launching

```bash
fcheap studio
```

The Studio requires an interactive terminal. It checks `term.IsTerminal()` on both stdin and stdout before launching. If run in a non-interactive context (piped input/output), it exits with an error message suggesting CLI commands instead.

## Views

### List View

The default view shows all stashes in a table:

- Stash ID
- Name
- Tool (e.g., vidtrace, manual)
- Tags
- File count
- Size
- Created date

Navigate with `j`/`k` and press `Enter` to open a stash's detail view.

### Detail View

Shows the full manifest for a selected stash:

- Metadata: ID, name, created, source, tool, bundle type
- File counts and sizes
- Content hash
- Tags
- File tree with sizes

### Help View

Press `?` to show keybindings.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Open detail view |
| `Esc` / `Backspace` | Go back |
| `m` | Toggle metadata |
| `o` | Open selected file externally |
| `r` | Reveal file in file manager |
| `c` | Copy path to clipboard |
| `q` | Quit |
| `?` | Show help |

## Platform Support

The Studio adapts external commands to the platform:

- **macOS**: `open` for files, `open -R` for reveal, `pbcopy` for clipboard
- **Linux**: `xdg-open` for files, `xdg-open` for reveal, `xclip`/`xsel` for clipboard
- **Windows**: `cmd /c start` for files, `explorer` for reveal

## Tech Stack

- [Bubbletea v2](https://github.com/charmbracelet/bubbletea) -- TUI framework
- [Lipgloss v2](https://github.com/charmbracelet/lipgloss) -- styling
- [Bubbles v2](https://github.com/charmbracelet/bubbles) -- components (viewport)

Import paths use `charm.land/` (the Charm project's custom domain).