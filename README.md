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
[MCP setup](https://file.cheap/integrations/mcp-clients) ·
[Artifact handoff](https://file.cheap/integrations/local-artifact-references)

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
vault on your machine. The repository also contains a public website and a
private single-owner artifact service for trusted product integrations; it is
not a hosted vault for end users.

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

The public page does not need storage credentials. Authenticated private
artifact routes initialize their Neon and Blob dependencies only after a
trusted caller requests them. Chalupa, Cairntrace, and Glyphrun exchange stable
credential-free artifact references while file.cheap owns immutable bytes and
their retention lifecycle.

`fcheap artifact-ref <stash-id> --json` and the `fcheap_artifact_ref` MCP tool
emit the same strict `fcheap-local` envelope with credential-free transport
fields for an existing stash. The local URI has no userinfo, query, fragment,
or dedicated credential field; this is not DLP for permitted identifiers,
paths, caller-supplied producer metadata, stash names, or tags exposed by other
CLI and MCP operations. The command is available in `v0.30.0` and later. See the
[local artifact handoff guide](https://file.cheap/integrations/local-artifact-references)
for Cairntrace, Glyphrun, and Chalupa examples.

### Private artifact service

`POST /api/v1/artifacts/plans` issues an exact short-lived private upload grant
that never outlives the requested retention timestamp.
After the direct upload, `POST /api/v1/artifacts/commits` records the immutable
object in Neon and returns an `ArtifactRefV1`. Administrator-only list, detail,
and download-plan routes never expose permanent storage credentials.
An identical plan can renew an expired transfer grant with the same idempotency
key, including after an ambiguous upload. Retention removes abandoned plans,
expired committed artifacts, and stale deletion leases without deleting a plan
that won a concurrent renewal. One failed object is isolated so the same
bounded run can still reconcile healthy candidates before reporting failure.
An exact replay of a committed plan returns its durable summary without another
grant, and its original receipt remains idempotent after transfer expiry while
the artifact is retained.

Vercel Private Blob is behind a provider port, so a future Spaces, R2, or S3
adapter does not change the API or `ArtifactRefV1` contract. Direct Blob uploads
use compressed chunks no larger than 2 MiB. The service reads each bounded
chunk after upload and verifies its SHA-256 before commit. Consumers still
verify complete downloads, and the local CLI never evicts a source stash because
a remote artifact exists. A signed download is never issued for an artifact
whose retention timestamp has passed, and no grant can outlive that timestamp.

Chalupa's Vercel service uses OIDC with an exact issuer, audience, and subject.
That identity may publish Chalupa log chunks and request a signed download for
one known committed `chalupa.log-chunk` that was produced by Chalupa with the
allowlisted native schema. Other artifact kinds remain administrator-only.
External publishers use independently rotated credentials bound to one exact
producer tool, artifact-kind allowlist, and native-schema allowlist. Those
credentials authorize only artifact plan and commit routes; they cannot list,
download, administer, or run retention.

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

Run a complete save → search → inspect workflow, then either hand off a stable
local pointer or restore the verified bytes:

```bash
# Check the local paths and optional integrations.
fcheap doctor

# Save and index any file or directory.
fcheap save ./agent-artifacts --tag bug-142 --tool my-agent --index

# Copy the returned stash ID, then find content across indexed stashes.
fcheap search "columns disappeared after refresh"
fcheap info <stash-id>

# Give another tool a versioned pointer without copying artifact bytes.
fcheap artifact-ref <stash-id> --json

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

- Go 1.25.12 or newer
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
