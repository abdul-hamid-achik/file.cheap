# Give Claude Code a local file vault with MCP

Coding agents are good at creating evidence and bad at deciding what should
remain on your disk. A single investigation can leave behind screenshots,
transcripts, test output, generated reports, and temporary patches spread across
several directories. Deleting everything loses context; keeping everything makes
the next investigation harder.

file.cheap gives Claude Code a small set of explicit file-vault operations over
the Model Context Protocol (MCP). The files remain on your machine. Claude can
save a snapshot, inspect its manifest, search indexed content, restore it to a
working directory, and remove it only after you confirm that the work is done.

## 1. Install the binary

On macOS:

```bash
brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap
```

On Linux, follow the current package command in the
[installation guide](/guide/getting-started#linux-with-a-debian-package). You can also build the
CGO-free binary directly:

```bash
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

Check the runtime before connecting an agent:

```bash
fcheap doctor
```

The default vault is under your XDG data directory. Change it with
[`fcheap config`](/cli/config) if you want the vault on another local volume.

## 2. Register the MCP server

Add file.cheap to Claude Code at user scope so it is available in every
project:

```bash
claude mcp add -s user fcheap -- fcheap mcp serve
```

The process uses stdio: Claude launches `fcheap mcp serve` locally and exchanges
typed MCP messages with it. There is no file.cheap account or API server in the
middle.

If you prefer a JSON configuration, use:

```json
{
  "mcpServers": {
    "fcheap": {
      "command": "fcheap",
      "args": ["mcp", "serve"]
    }
  }
}
```

See [MCP client setup](/integrations/mcp-clients) for Codex CLI, OpenCode, and
generic stdio clients.

## 3. Save useful evidence

Ask Claude to save a path only after the producing command has finished. A
precise request gives the stash useful provenance:

> Save `/tmp/login-repro` with file.cheap. Tag it `login` and `repro`, record
> `vidtrace` as the tool, and index it so we can search it later.

Claude calls `fcheap_save`; the equivalent terminal command is:

```bash
fcheap save /tmp/login-repro \
  --tag login \
  --tag repro \
  --tool vidtrace \
  --index
```

Every save gets an immutable stash ID, a manifest, per-file hashes, and an
overall content hash. The save-time scanner also looks for likely credentials.
It records the file, rule, and line of each finding, never the secret value.

## 4. Find the artifact again

Later, Claude can list recent stashes by tag or search the indexed files:

> Find the stash containing the message about a refresh token being rejected.
> Tell me which file matched before restoring anything.

The agent can read `fcheap://agent-guide` for its operating contract, read
`fcheap://stashes`, call `fcheap_search`, and then inspect the
single-stash resource. Search results name the matching file, not only the stash.
That distinction matters when one saved bundle contains hundreds of frames or
logs.

Keyword search stays entirely local. If you configure semantic or hybrid search,
read the [remote embedding safety notes](/cli/config#remote-embedding-safety):
OpenAI and non-loopback Ollama endpoints receive indexed text and queries.

## 5. Restore or connect it to code

Restore into a fresh temporary directory when you only need to inspect the
evidence:

```bash
fcheap restore <stash-id>
```

The command reports the generated path and verifies the restored files against
the manifest. For a bug investigation, `fcheap_connect` can instead use the
artifact's text to search a live codebase with vecgrep:

```bash
fcheap connect <stash-id> ~/projects/my-app --index
```

That returns ranked `file:line` candidates. It is a lead for the investigation,
not a claim that the highest-scoring file caused the bug.

## 6. Make deletion deliberate

The MCP drop tool requires `force: true`, and the CLI requires `--force`:

```bash
fcheap drop <stash-id> --force
```

For regenerable artifacts, set a TTL when saving and periodically run
[`fcheap sweep`](/cli/sweep). For evidence you may need later, keep the stash
until the investigation is closed. This explicit lifecycle is the useful part of
the vault: the agent can organize files, but it cannot silently decide that your
only copy is disposable.

Next, follow the complete [agent workflow examples](/guide/workflows) or review
all [MCP tools, resources, and prompts](/mcp/overview).
