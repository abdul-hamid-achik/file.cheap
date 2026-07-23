# docs

List and read the Markdown documentation embedded in every fcheap binary. From
a file.cheap source checkout, the same command group can also run the VitePress
development, build, and preview workflows through Bun.

For a concise operating contract intended for humans and agent harnesses, use
the top-level [`fcheap agent`](#agent-operating-guide) command.

## Usage

```bash
fcheap docs <subcommand> [flags]
```

## Subcommands

| Command | Description | Installed release |
|---|---|---:|
| `list` | List canonical embedded Markdown page paths | Yes |
| `show <page>` | Print one embedded page to stdout | Yes |
| `open` | Open `https://file.cheap/guide/` in the default browser | Yes |
| `serve` | Start the VitePress development server | Source checkout only |
| `build` | Build the production documentation site | Source checkout only |
| `preview` | Preview the built site locally | Source checkout only |

`list` and `show` read bytes compiled into the installed binary. They do not
depend on the caller's working directory, network access, Node.js, or Bun.

## List available pages

```bash
fcheap docs list
fcheap docs list --json
```

The human output prints sorted page paths such as `guide/getting-started.md` and
`cli/save.md`. The JSON form is useful when another tool needs to choose a page.

## Read a page

Pass a canonical path with or without its `.md` suffix:

```bash
fcheap docs show guide/getting-started
fcheap docs show guide/agent-guide.md
fcheap docs show cli/save
fcheap docs show mcp/overview
```

Absolute paths, traversal, and backslash-separated paths are rejected. Use
`--json` to receive the canonical page name and content as structured output.

## Agent operating guide

`fcheap agent` is a top-level read-only command, not a `docs` subcommand:

```bash
fcheap agent
fcheap agent --json
```

The default form prints a concise human operating guide. The JSON form emits a
versioned structured guide for agent harnesses. It describes the installed
version's capabilities, state-changing operations, remote embedding boundary,
failure handling, and destructive-action policy.

The complete public version is [Agent operating guide](/guide/agent-guide).

## Open the online site

```bash
fcheap docs open
```

This is the only installed docs subcommand that requires a graphical browser
and network access.

## Contributor site commands

The following commands require a file.cheap source checkout plus Bun:

```bash
fcheap docs serve
fcheap docs serve --port 8080 --open

fcheap docs build
fcheap docs build --output /tmp/fcheap-docs

fcheap docs preview
fcheap docs preview --port 8080 --open
```

| Subcommand | Flag | Default | Description |
|---|---|---|---|
| `serve` | `--port` | `5173` | Development-server port |
| `serve` | `--open` | `false` | Open a browser after startup |
| `build` | `--output` | `platform/docs/.vitepress/dist` | Production output directory |
| `preview` | `--port` | `4173` | Preview-server port |
| `preview` | `--open` | `false` | Open a browser after startup |

Contributors can run the VitePress scripts directly from `platform/docs/`:

```bash
bun install --frozen-lockfile
bun run docs:dev
bun run docs:build
bun run docs:preview
bun run docs:verify
```

The parent `platform/` project stages that build under `public/_docs` before
starting or building Next.js. Use `bun run docs:dev` here for VitePress-only
authoring, or `bun run dev` from `platform/` to test the complete site on one
origin.

The VitePress project and its dependencies are not bundled into release
packages. Installed users do not need Bun to run `docs list`, `docs show`, or
`fcheap agent`.

## MCP access

The MCP server exposes documentation in two complementary forms:

- resource `fcheap://agent-guide` for the concise operating guide;
- tool `fcheap_docs` for the guide, page listing, exact page reads, and site URL.

```json
{"action":"guide"}
{"action":"list"}
{"action":"show","page":"cli/save"}
{"action":"site"}
```

The server also includes the concise guide in its initialization instructions,
so a client receives the safety contract before choosing tools.

## See also

- [Agent operating guide](/guide/agent-guide)
- [CLI overview](/cli/)
- [MCP server reference](/mcp/overview)
