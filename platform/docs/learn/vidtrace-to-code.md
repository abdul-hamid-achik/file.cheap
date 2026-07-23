# From vidtrace evidence to the code that likely owns a bug

A screen recording contains the user's truth, but a codebase is organized by
components, routes, handlers, and symbols. The hard part of a visual bug
investigation is crossing that gap without losing the frame, transcript, and
provenance that explain what actually happened.

file.cheap turns a vidtrace bundle into a durable local stash, indexes its OCR
and transcript per file, and can hand the extracted evidence to vecgrep to rank
likely source locations. The final result is an investigation lead with a
`file:line`, not an automated root-cause verdict.

## 1. Extract the evidence

Create a vidtrace bundle from the source recording:

```bash
vidtrace extract ~/Downloads/checkout-bug.mp4 \
  --output /tmp/checkout-bug
```

A recognized bundle contains `metadata.json` and `timeline.json`. Its frames,
OCR, and transcript preserve details that a hand-written issue summary can
accidentally omit.

## 2. Save the completed bundle

Save only after extraction has finished so the stash is a coherent snapshot:

```bash
fcheap save /tmp/checkout-bug \
  --name 'Checkout refresh failure' \
  --tag checkout \
  --tag repro \
  --tool vidtrace \
  --source ~/Downloads/checkout-bug.mp4 \
  --index
```

file.cheap records the original source, file hashes, tool, tags, and secret-scan
findings in the manifest. It detects the vidtrace structure and extracts
searchable timeline text instead of treating the directory as an opaque video
dump.

## 3. Find the decisive moment

Search the saved files before opening every frame:

```bash
fcheap search 'payment columns disappeared after refresh' --mode keyword
```

The result points to a matching file and, for vidtrace content, the relevant
frame or timestamp when available. If the recording and your query use
different wording, configure an embedder and try
[`--mode hybrid`](/learn/bm25-semantic-hybrid-search).

## 4. Connect the evidence to a live repository

Install vecgrep separately and verify that file.cheap can find it:

```bash
fcheap doctor
fcheap connect <stash-id> ~/projects/storefront --index
```

With `--index`, file.cheap initializes and indexes the target repository before
searching it. It derives the query from the stash's OCR and transcript unless
you provide a narrower one:

```bash
fcheap connect <stash-id> ~/projects/storefront \
  --query 'table columns reset after token refresh' \
  --mode hybrid \
  --limit 10
```

The output is a ranked set of code chunks with score, path, line, and snippet.
Use it to choose where to inspect first. Confirm the hypothesis with the live
control flow, tests, logs, and the original frame.

## 5. Compare and restore without losing integrity

If the artifact also contains generated output that should match a working
directory, compare it first:

```bash
fcheap diff <stash-id> ~/projects/storefront/tmp/evidence
```

Restore into a fresh directory when you need the complete evidence:

```bash
fcheap restore <stash-id>
```

The restore verifies files against the saved manifest. That gives the next
engineer confidence that the evidence they inspect is the same snapshot that
drove the code search.

## 6. Close the lifecycle explicitly

Keep important evidence, assign a TTL to regenerable bundles, or remove the
stash only after the investigation is complete:

```bash
fcheap ttl <stash-id> 30d
fcheap sweep
```

`sweep` previews its plan by default. Explicit lifecycle commands are safer than
letting an agent silently decide that the only copy of a reproduction is no
longer useful.

For the exact flags and failure modes, read the
[`connect` reference](/cli/connect). The broader sequence appears in
[agent workflow examples](/guide/workflows).
