# Build a local-first agent stack

An agent workflow does not have to become a hosted application just because it
needs durable context. A useful local stack separates four jobs: the MCP client
decides what to do, file.cheap owns artifact lifecycle, veclite retrieves saved
files, and optional local model tools add semantic retrieval.

![Architecture diagram showing an MCP client calling the local fcheap process, which stores manifests, content, SQLite metadata, and a veclite index while optionally calling loopback Ollama and vecgrep.](/local-first-agent-stack.svg)

The default path stays inside one machine. Network access appears only when you
configure a remote embedder or independently copy a stash somewhere else.

A versioned `fcheap-local` artifact reference can cross process or product
boundaries as metadata, but it does not change that network boundary. The
reference resolves only where its source vault exists.

## The pieces

### MCP client

Claude Code, Codex CLI, OpenCode, or another stdio MCP client launches
`fcheap mcp serve`. It can call 15 typed tools, read stash resources, and use
investigation prompts. The client never needs direct access to the vault layout.

### file.cheap

The single CGO-free Go binary saves immutable snapshots, records provenance,
scans for likely secrets, compresses large payloads, verifies restores, and
manages retention. Its local manifest remains the portable source of truth.

### SQLite and veclite

SQLite is a queryable write-through index over manifests; it self-heals from the
on-disk stash directories. veclite keeps one search document per saved file and
provides BM25 without a model. Both indexes are derived local state, so they can
be rebuilt rather than treated as irreplaceable payloads.

### Ollama, optional

A loopback Ollama endpoint adds semantic vectors while keeping document and
query text on the machine. OpenAI and non-loopback Ollama endpoints are also
supported, but they expand the privacy boundary.

### vecgrep, optional

file.cheap searches saved evidence. vecgrep searches a live codebase. The
`connect` command bridges them by extracting a query from a stash and returning
ranked source `file:line` candidates. vecgrep remains a separately installed
runtime dependency.

## Assemble the stack

Install file.cheap and check its runtime:

```bash
brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap
fcheap doctor
```

Register the MCP server with your client using the
[copy-paste configurations](/integrations/mcp-clients). BM25 needs no further
setup. To add local semantic search:

```bash
fcheap config set embedder ollama
fcheap config set ollama_url http://127.0.0.1:11434
fcheap config set embed_model nomic-embed-text
```

Then save and index an artifact:

```bash
fcheap save /tmp/agent-evidence --tool vidtrace --tag repro --index
fcheap search 'where the table stopped updating' --mode hybrid
```

## Keep the boundaries honest

- The vault contains payloads and manifests; do not make a derived search index
  the only copy of anything.
- A save-time secret warning is a review aid, not a guarantee that content is
  safe to transmit.
- Loopback Ollama is local. OpenAI and non-loopback Ollama send indexed text and
  queries over the network.
- `connect` returns likely code ownership, not confirmed causality.
- Local-first is not the same as backed up. Valuable evidence still needs an
  independently designed recovery policy.

This shape stays small because every component has one job. Start with BM25 and
the MCP server; add Ollama only when paraphrase search improves real work, and
add vecgrep only when connecting evidence to source code is a repeated need.

Next, compare [BM25, semantic, and hybrid search](/learn/bm25-semantic-hybrid-search)
or follow the [local artifact reference integration](/integrations/local-artifact-references).
