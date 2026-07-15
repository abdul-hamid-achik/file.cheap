# Learn file.cheap

file.cheap is a local-first vault for the files that agent workflows create:
screenshots, logs, transcripts, generated reports, code maps, and investigation
evidence. These guides focus on the decisions behind the commands, so you can
build a workflow that stays searchable without turning every artifact into a
permanent folder on your machine.

## Start here

- [Give Claude Code a local file vault](/learn/claude-code-local-file-vault) —
  install fcheap, register its MCP server, and let an agent save and retrieve
  artifacts without leaving the conversation.
- [Local-first vs cloud agent artifacts](/learn/local-first-vs-cloud-artifacts) —
  choose where files should live based on privacy, recovery, collaboration, and
  cost rather than habit.
- [BM25, semantic, and hybrid file search](/learn/bm25-semantic-hybrid-search) —
  understand what each search mode can find and when a local embedder is worth
  running.
- [From vidtrace evidence to owning code](/learn/vidtrace-to-code) — turn a
  recorded bug into searchable evidence and ranked source-code candidates.
- [Build a local-first agent stack](/learn/local-first-agent-stack) — see how
  fcheap, MCP, Ollama, and optional vecgrep fit together without a hosted
  application.
- [MCP tools cheat sheet](/learn/mcp-tools-cheat-sheet) — choose the smallest
  safe tool, resource, or prompt for an agent workflow.
- [Agent operating guide](/guide/agent-guide) — give an assistant the
  version-matched safety and tool-selection contract also printed by
  `fcheap agent`.

## Comparisons

- [file.cheap vs Git stash and worktree](/compare/git-stash-worktree)
- [file.cheap vs cloud artifact storage](/compare/cloud-artifact-storage)

If you already know the workflow you want, go directly to the
[getting-started guide](/guide/getting-started), the [CLI reference](/cli/),
or the [MCP client setup guide](/integrations/mcp-clients).
