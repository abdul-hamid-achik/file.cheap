# Publish Chalupa agent artifacts from a laptop

Chalupa runs open models on ephemeral GPU droplets and launches coding agents
against them. After a turn, the Chalupa CLI on the operator's laptop produces a
small JSON **inference receipt**: model tag, optional digest, context size,
status, token counts, tool-call counts, and timings. It never contains prompts,
completions, or file contents.

This page is the exact contract for storing those receipts — and optional
redacted session transcripts — in the private file.cheap artifact service from a
machine that has no Vercel OIDC identity.

Nothing here is a new API. The laptop uses the same plan → direct PUT → commit
flow, the same idempotency rules, and the same
[`ArtifactRefV1`](/integrations/local-artifact-references) as every other
producer. What Chalupa needs is one producer-bound publisher credential.

## Authorized kinds

| Kind | Native schema | Content type | Max size |
|---|---|---|---:|
| `chalupa.inference-receipt` | `urn:chalupa:inference-receipt:v1` | `application/json` | 256 KiB |
| `chalupa.agent-session` | `urn:chalupa:agent-session:v1` | `application/json` | 8 MiB |

Both are published by producer tool `chalupa-cli`. That producer is distinct
from the Vercel-side `chalupa` OIDC identity, which keeps its own CI kinds; a
laptop credential can never publish or read a CI artifact, and the CI identity
can never publish a receipt.

The pairs are exact. A plan that mixes the receipt kind with the session schema
is rejected with `401`, not silently accepted.

Retention is chosen per artifact by the caller through `expiresAt`, anywhere
between now and 31 days from now. Omitting it retains the artifact until it is
deleted from the console. Redact a session transcript before publishing it: the
service stores immutable bytes and never inspects, opens, or filters them.

## Server-side policy

The producer is one entry in the platform's publisher keyring. It is
configuration, not code:

```jsonc
// FILECHEAP_PUBLISHER_TOKENS (compact JSON, in Vercel; never in a repository)
{
  "chalupa-cli": {
    "kindSchemaBindings": [
      {
        "kind": "chalupa.inference-receipt",
        "maxSizeBytes": 262144,
        "nativeSchema": "urn:chalupa:inference-receipt:v1"
      },
      {
        "kind": "chalupa.agent-session",
        "maxSizeBytes": 8388608,
        "nativeSchema": "urn:chalupa:agent-session:v1"
      }
    ],
    "maxSizeBytes": 8388608,
    "tokens": ["<43-128 character base64url credential>"]
  }
}
```

Each binding is one exact `kind` ↔ `native_schema` pair with its own byte
quota. A binding quota may narrow the producer's `maxSizeBytes`, never widen it.
`tokens` holds the current credential and, only during rotation, the next one.

To add a third kind, append one more binding and redeploy the platform. No code
changes and no migrations are involved.

## Credentials

The laptop process receives **only its own credential**, as
`FILECHEAP_INGEST_TOKEN`. Load it from TinyVault into the publisher process
alone. Never place it on a command line, in logs, in artifact metadata, in a
receipt, in a sidecar, or in a child process.

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap
FILECHEAP_INGEST_TOKEN='<the chalupa-cli publisher credential>'
```

Every request below sends it as `Authorization: Bearer $FILECHEAP_INGEST_TOKEN`.
Request bodies are JSON and must carry `Content-Type: application/json`; a
control-plane body above 16 KiB is rejected with `413`. Responses always carry
`X-Request-Id`; echo it in any support request.

## The shortest path: `fcheap publish`

If the laptop already has `fcheap`, the whole flow is one command and no HTTP
code:

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
FILECHEAP_INGEST_TOKEN='…' \
fcheap publish ./inference-receipt.json \
  --content-type application/json \
  --kind chalupa.inference-receipt \
  --producer-tool chalupa-cli \
  --native-schema urn:chalupa:inference-receipt:v1 \
  --native-id "$CHALUPA_RUN_ID" \
  --expires-in 720h \
  --json
```

It streams the file once to hash it, plans, PUTs, commits, and prints a
`filecheap-publish/1` receipt containing the `ArtifactRefV1`. Do not pass
`--run-index`: a turn receipt is not a Cairntrace or Glyphrun run. See
[`fcheap publish`](/cli/publish) for the full flag reference.

The rest of this page is the same protocol for a Chalupa CLI that speaks HTTP
itself.

## 1. Plan the upload

```http
POST /api/v1/artifacts/plans
Authorization: Bearer $FILECHEAP_INGEST_TOKEN
Content-Type: application/json
```

```json
{
  "contentType": "application/json",
  "expiresAt": "2026-08-06T00:00:00.000Z",
  "idempotencyKey": "6f1e4a2c-6b31-4f2f-9a1a-2c9c0a4a1d55",
  "kind": "chalupa.inference-receipt",
  "producer": {
    "tool": "chalupa-cli",
    "version": "0.14.2",
    "native_schema": "urn:chalupa:inference-receipt:v1",
    "native_id": "run-2026-08-05-17-42-01"
  },
  "sha256": "0d1c…64 lowercase hex characters…9ab",
  "sizeBytes": 1284
}
```

| Field | Rules |
|---|---|
| `contentType` | Media type without parameters. Must equal the bytes you PUT. |
| `expiresAt` | Optional RFC 3339 timestamp, after now and at most 31 days ahead. |
| `idempotencyKey` | UUID, lowercased by the service. It derives the artifact ID. |
| `kind` | One of the two kinds above. |
| `producer.tool` | Exactly `chalupa-cli`. |
| `producer.version` | Optional. |
| `producer.native_schema` | The schema bound to `kind`. Required for an authorized publish. |
| `producer.native_id` | Optional producer-native ID, ≤ 160 characters. Use the Chalupa run ID. |
| `producer.entrypoint` | Optional safe relative path inside the artifact. |
| `sha256` | Lowercase hex digest of the exact bytes. |
| `sizeBytes` | Exact byte count. Bounded by the kind quota and the 64 MiB platform ceiling. |

`201 Created` returns the plan:

```json
{
  "artifact": {
    "artifactId": "art_6f1e4a2c6b314f2f9a1a2c9c0a4a1d55",
    "committedAt": null,
    "contentType": "application/json",
    "expiresAt": "2026-08-06T00:00:00.000Z",
    "kind": "chalupa.inference-receipt",
    "producer": { "…": "as sent" },
    "sha256": "0d1c…9ab",
    "sizeBytes": 1284,
    "state": "planned",
    "verification": "server-sha256"
  },
  "artifactRef": { "…": "see below" },
  "receipt": "8b0f…-uuid",
  "upload": {
    "expiresAt": "2026-08-05T18:00:00.000Z",
    "headers": { "content-type": "application/json" },
    "method": "PUT",
    "url": "https://…short-lived signed URL…"
  }
}
```

`200 OK` instead means this exact plan already committed. The body is the
artifact summary with `state: "committed"` and **no** `upload` and **no**
`receipt`; treat it as success and stop.

The `receipt` and `upload.url` are transfer capabilities. Keep the receipt in
Chalupa's protected delivery state only, never in the sidecar, output, or logs.
The signed URL must never be printed or persisted.

## 2. PUT the exact bytes

Send the identical bytes to `upload.url` with `upload.method` and every header
in `upload.headers` — nothing more:

```http
PUT <upload.url>
Content-Type: application/json
```

The grant is exact-path, non-overwrite, and constrained to the planned size and
content type; it lives at most 15 minutes and never past `expiresAt`. Never
retry the PUT automatically: its outcome can be ambiguous. If storage answers
with a non-overwrite conflict, the bytes are already there — go straight to
commit, which verifies them server-side.

## 3. Commit

```http
POST /api/v1/artifacts/commits
Authorization: Bearer $FILECHEAP_INGEST_TOKEN
Content-Type: application/json
```

```json
{ "receipt": "8b0f…-uuid" }
```

The service rechecks the producer, kind, native schema, and quota, then streams
the stored object and recomputes its SHA-256 before returning `200 OK` with the
artifact summary and `"verification": "server-sha256"`. Repeating the same
receipt is idempotent while the artifact is retained and the plan has not
expired.

## 4. Download it back

A publisher credential can read back only what its own policy authorizes it to
write: same `producer.tool`, same kind, same native schema. Any other artifact —
including another producer's — is reported as `404 artifact_not_found` before
private storage is touched.

```http
POST /api/v1/artifacts/downloads
Authorization: Bearer $FILECHEAP_INGEST_TOKEN
Content-Type: application/json
```

```json
{ "artifactId": "art_6f1e4a2c6b314f2f9a1a2c9c0a4a1d55" }
```

`201 Created` returns the artifact summary plus a grant:

```json
{
  "download": {
    "expiresAt": "2026-08-05T18:00:00.000Z",
    "headers": {},
    "method": "GET",
    "url": "https://…short-lived signed URL…"
  }
}
```

The grant lasts at most 15 minutes and never past the artifact's retention
timestamp. Verify the SHA-256 of the downloaded bytes yourself; a storage `HEAD`
or ETag is not a verification claim.

## The sidecar Chalupa stores

Persist the credential-free `artifactRef` from the plan or commit response next
to the local receipt. It is the durable, shareable pointer:

```json
{
  "$schema": "urn:filecheap.dev:artifact-ref:v1",
  "version": 1,
  "provider": "fcheap-cloud",
  "uri": "fcheap://cloud/vaults/private/artifacts/art_6f1e4a2c6b314f2f9a1a2c9c0a4a1d55",
  "artifact_id": "art_6f1e4a2c6b314f2f9a1a2c9c0a4a1d55",
  "kind": "chalupa.inference-receipt",
  "producer": {
    "tool": "chalupa-cli",
    "native_schema": "urn:chalupa:inference-receipt:v1",
    "native_id": "run-2026-08-05-17-42-01"
  }
}
```

It contains no credential, no receipt, and no signed URL, so it is safe in a
repository, a run log, or a message to another agent.

## Errors

Every failure is RFC 9457 `application/problem+json` with `code`, `title`,
`detail`, `status`, `instance`, and `requestId`.

| Status | `code` | Meaning and action |
|---:|---|---|
| 400 | `invalid_json` | The body was not JSON. |
| 400 | `invalid_receipt` | The receipt is unknown, expired, or belongs to another producer. Re-plan with the same idempotency key. |
| 401 | `unauthorized` | Missing credential, or a kind, schema, or producer tool outside this token's policy. Do not retry with a different kind. |
| 404 | `artifact_not_found` | No committed artifact this credential may read. |
| 409 | `idempotency_conflict` | The key is bound to different metadata or content. Use a new key. |
| 409 | `idempotency_reconciling` | An expired plan is being reconciled. Wait for `Retry-After` and repeat the same plan. |
| 409 | `upload_incomplete` | The PUT never landed. Re-plan and transfer again. |
| 409 | `commit_conflict` | The plan expired or entered retention mid-commit. Re-plan with the same key. |
| 413 | `producer_quota_exceeded` | Larger than this producer's quota for that kind. The detail names the producer, the kind, and the exact quota. |
| 413 | `payload_too_large` | The JSON request body itself exceeded 16 KiB. |
| 415 | `unsupported_media_type` | Send `Content-Type: application/json`. |
| 422 | `invalid_request` | The body failed schema validation. |
| 422 | `integrity_mismatch` | The stored object does not match the planned size, content type, or digest. |
| 422 | `artifact_retention_expired` | `expiresAt` is in the past or the window already closed. |
| 503 | — | Transient. Honor `Retry-After`. |

## Retry rules

- Retry the **plan** with the same idempotency key. Persist the normalized plan
  facts and reuse the original `expiresAt`; never recompute a rolling retention
  date on retry.
- Retry the **commit** with the same receipt until the plan expires. Afterward,
  repeat the idempotency-keyed plan instead.
- Never retry the **PUT** automatically.
- The local receipt file is never modified, moved, or deleted by any failure.

## Boundaries

- Publish receipts, not prompts. The service stores what you send, forever
  immutable, until retention deletes it.
- Never send a Vercel, Blob, database, administrator, or cron credential to
  these routes; they are rejected and belong to different boundaries entirely.
- One credential, one producer. Rotating means activating the next token,
  updating the vault value, verifying a publish, and retiring the previous one —
  no client change.
