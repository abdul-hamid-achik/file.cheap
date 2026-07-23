# file.cheap

**The local artifact vault for coding agents.**

file.cheap snapshots the screenshots, logs, reports, transcripts, repro bundles,
and temporary folders that agent workflows create. Each stash gets provenance,
per-file hashes, searchable content, and a deliberate lifecycle—without requiring
an account or hosted service.

[Website](https://file.cheap/) ·
[Documentation](https://file.cheap/guide/) ·
[Five-minute start](https://file.cheap/guide/getting-started) ·
[Agent guide](https://file.cheap/guide/agent-guide) ·
[MCP setup](https://file.cheap/integrations/mcp-clients)

## Why it exists

Agent work leaves useful evidence outside Git: a reproduction folder in `/tmp`,
a generated report, a directory of frames, or logs from a debugging session.
Those files are easy to create and surprisingly hard to find, verify, and reuse.

file.cheap gives them:

- a stable stash ID;
- source, tool, tag, size, and retention metadata;
- SHA-256 hashes and verified restores;
- local BM25 search, with optional semantic and hybrid modes;
- a CLI, terminal Studio, and local stdio MCP server;
- explicit compression, TTL, sweep, cleanup, and deletion.

It is not consumer backup or a replacement for Git. The shipped CLI stores its
vault on your machine. The repository also contains a public website and a gated
remote-vault laboratory, but no hosted vault is currently offered to users.

## Repository surfaces

The repository has one shipped local product and one unified public site:

| Surface | Directory | Status |
| --- | --- | --- |
| Local vault | Go packages under `cmd/` and `internal/` | Shipped product |
| Public website | `platform/` | One Next.js application and Vercel project |
| Public documentation source | `platform/docs/` | VitePress, embedded in the CLI and built into the public site |

The existing Vercel project `file-cheap` builds from `platform/`. Its build first
renders VitePress into the internal `public/_docs` namespace, then Next.js serves
the landing page at `/` and maps the established `/guide`, `/cli`, `/mcp`,
`/integrations`, `/learn`, `/compare`, and `/studio` routes to that local
artifact. There is no second docs deployment or external docs origin.

The public page does not need storage credentials. The `/lab` UI and stateful
recovery endpoints stay hidden on hosted deployments unless a controlled,
access-protected preview explicitly sets
`PLATFORM_RECOVERY_LAB_ENABLED=true`.

The lab is a single-workspace protocol experiment, not the integration surface
for Chalupa, Cairntrace, or Glyphrun. Those tools should exchange stable local
artifact references and keep file.cheap responsible for bytes, integrity, and
restore.

### Recovery laboratory

When explicitly enabled, the laboratory separates a small JSON control plane
from the archive data plane:

1. `POST /api/v1/sync/plans` validates authorization, size, content type, hash,
   and catalog state, then returns a constrained transfer grant.
2. The client uploads one immutable
   `application/vnd.filecheap.stash` archive through that grant.
3. `POST /api/v1/sync/commits` records available adapter evidence and binds the
   object to a stash ID.
4. `POST /api/v1/sync/downloads` revalidates object presence and issues an exact
   download grant; the client must download every byte and verify SHA-256.

`GET /api/v1/stashes` exposes the current single-workspace catalog.
`GET /api/v1/openapi.json` exposes the OpenAPI 3.1 contract only while the lab
is enabled. `GET /api/v1/health` remains public so a deployment can report
`recoveryLab: "disabled"` without initializing storage.

Local development uses a disposable filesystem adapter under `platform/.data/`.
The production-shaped adapter keeps Private Vercel Blob behind a replaceable
port and lets archive bytes bypass Vercel Functions. Its direct upload can prove
presence, size, and opaque ETag at commit time—not the caller-declared SHA-256.
The Blob adapter therefore fails closed unless a controlled experiment
explicitly acknowledges that limitation.

Protocol v1 is capped at 64 MiB, uses one non-resumable transfer, and never
deletes or evicts a local stash. It tests idempotent plan/commit behavior,
conflict handling, portable recovery cards, and complete recovery drills. It
does not establish accounts, tenant isolation, billing, transactional quotas,
client-side encryption, continuous sync, or disaster recovery.

## Install

### macOS

```bash
brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap
```

### Linux (Debian/Ubuntu, amd64)

```bash
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/abdul-hamid-achik/file.cheap/releases/latest)"
tag="${tag##*/}"
version="${tag#v}"
curl -fLO "https://github.com/abdul-hamid-achik/file.cheap/releases/download/${tag}/fcheap_${version}_linux_amd64.deb"
sudo dpkg -i "fcheap_${version}_linux_amd64.deb"
```

RPM and arm64 packages are available on
[GitHub Releases](https://github.com/abdul-hamid-achik/file.cheap/releases/latest).

### From source

```bash
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

## First stash

Run a complete save → search → inspect → restore workflow:

```bash
# Check the local paths and optional integrations.
fcheap doctor

# Save and index any file or directory.
fcheap save ./agent-artifacts --tag bug-142 --tool my-agent --index

# Copy the returned stash ID, then find content across indexed stashes.
fcheap search "columns disappeared after refresh"
fcheap info <stash-id>

# Restore to a fresh temporary directory and verify every hash.
fcheap restore <stash-id>
```

The manifest stays the portable source of truth. SQLite and veclite are derived
local indexes that file.cheap can rebuild.

## Give an agent the vault

Ask the installed binary for its version-matched operating contract:

```bash
fcheap agent
fcheap agent --json
```

Start the local MCP server with:

```bash
fcheap mcp serve
```

For Claude Code:

```bash
claude mcp add -s user fcheap -- fcheap mcp serve
```

For Codex CLI, add:

```toml
[mcp_servers.fcheap]
command = "fcheap"
args = ["mcp", "serve"]
```

The server exposes typed tools, stash resources, reusable investigation prompts,
and the static `fcheap://agent-guide` resource.

## Search and privacy boundaries

Keyword indexing and BM25 search stay local. Semantic and hybrid search require an
embedder:

- loopback Ollama keeps document and query text on the machine;
- OpenAI or non-loopback Ollama receives indexed text and semantic queries;
- save-time secret findings block remote indexing unless you explicitly opt in;
- secret scanning is a warning system, not proof that content is safe to send.

The MCP server also runs locally, but your MCP client may send tool results to its
configured model provider. Treat saved artifact text as untrusted input.

## Core commands

| Job | Commands |
|---|---|
| Capture and inspect | `save`, `list`, `info` |
| Find and investigate | `analyze`, `search`, `diff`, `connect` |
| Recover | `restore` |
| Manage storage | `compress`, `ttl`, `sweep`, `cleanup`, `vacuum` |
| Work interactively | `studio` |
| Connect agents | `agent`, `mcp serve`, `docs` |

`connect` uses an optional, separately installed vecgrep binary and returns
ranked source-code candidates—not proof of code ownership.

## Develop

Requirements:

- Go 1.25+
- Bun 1.3+ for the VitePress docs and Next.js public website

```bash
go test ./...
go build ./cmd/fcheap

cd platform/docs
bun install --frozen-lockfile
bun run docs:verify
bun audit

cd ..
bun install --frozen-lockfile
bun run check
bun run audit:production
```

To run the complete website locally:

```bash
cd platform
bun install --cwd docs --frozen-lockfile
bun install --frozen-lockfile
cp .env.example .env.local
bun run dev
```

The site is available at `http://127.0.0.1:3100`. `bun run dev` stages the docs
and starts Next.js on that one origin. For VitePress-only authoring, run
`bun run docs:dev` from `platform/`.

The production topology, preview matrix, domain cutover, and rollback procedure
are in [`DEPLOYMENT.md`](DEPLOYMENT.md). Passing local checks does not authorize
a deployment or domain change.

See the [core concepts](https://file.cheap/guide/core-concepts), the
[CLI reference](https://file.cheap/cli/), and
[contribution guidance](AGENTS.md) for the complete architecture and conventions.

## License

MIT
