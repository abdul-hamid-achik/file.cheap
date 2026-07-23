# BM25, semantic, and hybrid search for your own files

Searching an artifact vault is different from searching the web. You often know
an exact error code, filename, or log fragment, but sometimes you remember only
the meaning: "the report where the login loop started after a refresh." BM25 and
vector search solve different halves of that problem. Hybrid search combines
them, but it is useful only when you understand what each score represents.

file.cheap keeps one search document per saved file in its local veclite index.
BM25 works without any model. Semantic and hybrid modes become available when
you configure an Ollama or OpenAI embedder.

## BM25: exact language wins

BM25 is a keyword-ranking algorithm. It rewards documents that contain the
query terms, gives uncommon terms more weight, and adjusts for document length.
It is usually the best first choice for:

- error codes such as `ECONNRESET` or `OPG-15061`;
- function, field, and command names;
- quoted log fragments;
- filenames and product-specific terminology;
- searching a vault without running an embedding model.

```bash
fcheap search 'refresh_token expired' --mode keyword
```

BM25 does not understand that "credential renewal failed" may describe the same
event as "refresh token rejected" when those words never appear together.

## Semantic search: meaning wins

Semantic search embeds the query and each file into vectors, then ranks files by
meaning rather than exact token overlap. It is helpful for:

- paraphrased symptoms;
- OCR or transcript text with inconsistent wording;
- broad questions such as "where did the checkout stop progressing?";
- finding conceptually related notes across tools and projects.

Configure a local Ollama embedder before indexing:

```bash
fcheap config set embedder ollama
fcheap config set embed_model nomic-embed-text
fcheap analyze <stash-id>
fcheap search 'session renewal stopped working' --mode semantic
```

Embedding is not magic. Very short identifiers, unusual hashes, and exact error
strings often rank worse than they do with BM25. A semantic match is also a
lead, not proof that two files describe the same incident.

## Hybrid search: keep both signals

Hybrid search asks both indexes and merges their ranked results. It is a strong
default when a query mixes exact language with intent:

```bash
fcheap search 'OPG-15061 columns disappeared after refresh' --mode hybrid
```

The ticket ID gives BM25 a precise anchor while the rest of the sentence gives
the vector index useful meaning. In a mixed vault, file.cheap also keeps BM25
results from stashes that were indexed before vectors were configured, so one
older stash does not silently disappear from the answer.

## Choose a mode deliberately

| You remember | Start with | Why |
|---|---|---|
| Exact error, symbol, ID, or phrase | `keyword` | Literal terms are the strongest evidence |
| Meaning but not wording | `semantic` | Embeddings can bridge paraphrases |
| Some exact terms plus a natural-language symptom | `hybrid` | Preserves both signals |
| No embedder is configured | `keyword` | Fully local and available by default |
| The target is source code, not saved files | [`fcheap connect`](/cli/connect) | vecgrep searches the live codebase |

## Privacy boundary

BM25 indexing and search remain local. Loopback Ollama also remains on the
machine. OpenAI and non-loopback Ollama endpoints receive file text during
indexing and query text during semantic or hybrid searches.

The save-time scanner blocks remote indexing of a stash that already has a
secret finding unless you explicitly set `allow_remote_secrets: true`. Query
text is not scanned by that guard. Review the
[embedding safety settings](/cli/config#remote-embedding-safety) before pointing
an embedder at a network endpoint.

## A repeatable search workflow

1. Save with useful tool and tag provenance.
2. Run `fcheap analyze <stash-id>` once the artifact is complete.
3. Begin with the most specific words you remember.
4. Use keyword mode for identifiers, hybrid mode for mixed queries, and
   semantic mode when you truly need paraphrase matching.
5. Inspect the returned filename and snippet before restoring the whole stash.
6. Use [`fcheap connect`](/cli/connect) only when the next question is which
   source file may own the behavior.

See the full [`search` command reference](/cli/search) for configuration,
fallback behavior, and JSON output, or follow the
[artifact investigation workflow](/guide/workflows) end to end.
