# docs

Read the documentation embedded in every fcheap binary. From a file.cheap source
checkout, you can also serve and build the VitePress site.

## Usage

```bash
fcheap docs <subcommand> [flags]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `serve` | Start a local VitePress dev server (source checkout required) |
| `build` | Build the docs site for production (source checkout required) |
| `preview` | Preview the built docs site locally (source checkout required) |
| `list` | List all embedded doc pages |
| `show <page>` | Print an embedded doc page to stdout |
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

The `fcheap_docs` MCP tool provides the read-only embedded subset for AI agents:

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

`list`, `show`, and `open` work from an installed release and do not require
Node.js. The Markdown pages are embedded in the binary.

`serve`, `build`, and `preview` require a file.cheap source checkout plus Node.js
and npm because the VitePress project and its build dependencies are not bundled
in release packages. When `node_modules` is absent, fcheap runs `npm ci` from the
committed `docs/package-lock.json` before starting the command.
