# `fcheap publish`

`fcheap publish` is an opt-in bridge from one bounded local file to the private
file.cheap artifact service. It does not save a stash, delete the input, or
accept Vercel, Blob, Neon, or administrator credentials.

The command reads one regular file of at most 2 MiB, hashes those exact bytes,
plans an upload, sends the bytes directly to the signed storage URL, and commits
only after the service reports `server-sha256` verification. Its JSON result is
a strict `filecheap-publish/1` receipt containing a credential-free
`fcheap-cloud` `ArtifactRefV1`.

## Usage

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
FILECHEAP_INGEST_TOKEN='producer-bound base64url credential' \
fcheap publish ./run.tar.gz \
  --content-type application/gzip \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --native-schema urn:cairntrace.dev:run:v1 \
  --json
```

`FILECHEAP_INGEST_TOKEN` is the only accepted secret. It must be a 43-128
character base64url credential assigned to exactly one producer. The
`--producer-tool`, `--kind`, and `--native-schema` values must match that
producer's server-side policy; a token for Cairntrace cannot publish a Glyphrun
or Chalupa artifact. The command rejects a Vercel runtime or OIDC credential
environment. Vercel-to-Vercel producers use OIDC directly with the service
rather than invoking this local command.

Load the token from TinyVault into only the publisher process. Do not put it in
the command line, logs, artifact metadata, a receipt, or a child process. Token
rotation does not change the CLI: an operator activates the next credential for
that producer, updates its TinyVault value, verifies publication, and retires
the previous credential.

## Retry behavior

The plan request may be retried once with one client-generated idempotency key;
the final commit may be retried with the same opaque service receipt. Direct PUT
is never retried automatically because its result can be ambiguous and a signed
URL must never appear in output. If the immutable object already exists, a
non-overwrite conflict is safe to advance to commit because the service reads
the bounded private bytes and verifies their SHA-256 before committing. The
local source remains unchanged in every failure case.

Protocol integrations that persist their own deterministic idempotency key may
repeat the exact plan after its up-to-15-minute transfer grant expires. A plan
and its upload grant are always capped at the artifact retention timestamp.
The service renews the grant for the same artifact and rejects any metadata or
content change. Once retention begins, it rejects a new grant or commit,
including a commit whose object verification crossed that boundary. An expired
plan that is currently being reconciled returns a retryable `409`; the caller
must wait and retry the same plan. The `fcheap publish` command keeps its
generated key only for the lifetime of one invocation.

Persist the normalized plan facts and reuse the original `expiresAt`; do not
recompute a rolling retention date on retry. Persist the opaque receipt only in
the producer's protected delivery state, never in `ArtifactRefV1`, logs, or
user-facing output. If the exact artifact already committed, a repeated plan
returns `200` with its committed summary and no upload grant. Repeating the
original receipt also returns the verified committed summary after the former
transfer grant expires, until artifact retention begins.

## JSON receipt

```json
{
  "version": "filecheap-publish/1",
  "artifact_ref": {
    "$schema": "urn:filecheap.dev:artifact-ref:v1",
    "version": 1,
    "provider": "fcheap-cloud",
    "uri": "fcheap://cloud/vaults/private/artifacts/art_example",
    "artifact_id": "art_example",
    "kind": "chalupa.log-chunk"
  },
  "sha256": "...",
  "size_bytes": 1234,
  "verification": "server-sha256",
  "published_at": "2026-07-24T00:00:00Z"
}
```

The receipt never contains the plan receipt, a signed URL, or credentials.
