# docs

Manage and serve the fcheap documentation site (VitePress).

## Usage

```bash
fcheap docs <subcommand> [flags]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `serve` | Start a local VitePress dev server |
| `build` | Build the docs site for production |
| `preview` | Preview the built docs site locally |
| `list` | List all available doc pages |
| `show <page>` | Print a doc page to stdout |
| `open` | Open the online docs site in a browser |

## Examples

### Serve docs locally

```bash
fcheap docs serve
fcheap docs serve --port 8080 --open
```

### Build docs for production

```bash
fcheap docs build
fcheap docs build --output /tmp/dist
```

### Preview the built docs

```bash
fcheap docs build
fcheap docs preview --open
```

### List available doc pages

```bash
fcheap docs list
fcheap docs list --json
```

### Read a doc page to stdout

```bash
fcheap docs show guide/getting-started
fcheap docs show cli/save
fcheap docs show mcp/overview
```

### Open the online docs site

```bash
fcheap docs open
```

## Flags

### serve

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `5173` | Port for the dev server |
| `--open` | bool | `false` | Open browser on start |

### build

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `docs/.vitepress/dist` | Output directory |

### preview

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `4173` | Port for the preview server |
| `--open` | bool | `false` | Open browser on start |

## MCP Tool

The `fcheap_docs` MCP tool provides the same functionality for AI agents:

| Action | Description |
|--------|-------------|
| `list` | List all available doc pages |
| `show` | Read a specific doc page (requires `page` argument) |
| `site` | Get the docs site URL and local serve command |

```json
{"action": "list"}
{"action": "show", "page": "cli/save"}
{"action": "site"}
```

## Requirements

The `serve`, `build`, and `preview` subcommands require Node.js and npm. If `node_modules` is not installed in the `docs/` directory, `fcheap docs serve` and `fcheap docs build` will automatically run `npm install` first.