import { createHash } from "node:crypto";

import { expect, test } from "bun:test";

import { VercelPrivateBlobArtifactStore, type ArtifactBlobSdk } from "@/platform/artifacts/vercel-blob-store";
import type { PlatformConfig } from "@/shared/config/env";

const config: PlatformConfig = { adminToken: "a".repeat(32), cronSecret: "c".repeat(32), databaseUrl: "postgresql://runtime", ownerAccountId: "acc_owner123", publisherTokens: [], publicUrl: "https://file.cheap" };

test("Vercel private Blob upload grants are exact, bounded, and non-overwrite", async () => {
  const calls: unknown[] = [];
  const key = "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  const blob = {
    issueSignedToken: async (input: unknown) => {
      calls.push(input);
      return {
        clientSigningToken: "client-signing-token",
        delegationToken: "delegation",
        validUntil: Date.now() + 60_000,
      };
    },
    presignUrl: async (_token: unknown, input: unknown) => {
      calls.push(input);
      return {
        presignedUrl:
          `https://vercel.com/api/blob/?pathname=${encodeURIComponent(key)}` +
          "&vercel-blob-delegation=delegation&vercel-blob-signature=signature",
      };
    },
  } as unknown as ArtifactBlobSdk;
  const store = new VercelPrivateBlobArtifactStore(config, blob);
  const validUntil = new Date("2026-07-24T00:15:00.000Z");
  const grant = await store.issueUploadGrant({ contentType: "application/zstd", key, sizeBytes: 1024, validUntil });
  expect(grant).toMatchObject({
    method: "PUT",
    url: expect.stringMatching(/^https:\/\/vercel\.com\/api\/blob\//),
  });
  expect(calls).toEqual([
    expect.objectContaining({ allowedContentTypes: ["application/zstd"], maximumSizeInBytes: 1024, operations: ["put"], pathname: "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", validUntil: validUntil.getTime() }),
    expect.objectContaining({ addRandomSuffix: false, allowOverwrite: false, maximumSizeInBytes: 1024, operation: "put", pathname: "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", validUntil: validUntil.getTime() }),
  ]);
});

test("Vercel private Blob download grants stay on the exact private object", async () => {
  const key = "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  const blob = {
    issueSignedToken: async () => ({
      clientSigningToken: "client-signing-token",
      delegationToken: "delegation",
      validUntil: Date.now() + 60_000,
    }),
    presignUrl: async () => ({
      presignedUrl:
        `https://store_abc123.private.blob.vercel-storage.com/${key}` +
        "?cache=0&vercel-blob-delegation=delegation&vercel-blob-signature=signature",
    }),
  } as unknown as ArtifactBlobSdk;
  const store = new VercelPrivateBlobArtifactStore(config, blob);
  const grant = await store.issueDownloadGrant({
    key,
    validUntil: new Date("2026-07-24T00:15:00.000Z"),
  });

  expect(grant).toMatchObject({
    headers: {},
    method: "GET",
    url: expect.stringMatching(
      /^https:\/\/store_abc123\.private\.blob\.vercel-storage\.com\//,
    ),
  });
});

test("Vercel private Blob rejects host, query, or delegation drift", async () => {
  const key = "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  for (const presignedUrl of [
    "https://attacker.example/private-object?cache=0&vercel-blob-delegation=delegation&vercel-blob-signature=signature",
    `https://store_abc123.private.blob.vercel-storage.com/${key}?cache=0&unexpected=1&vercel-blob-delegation=delegation&vercel-blob-signature=signature`,
    `https://store_abc123.private.blob.vercel-storage.com/${key}?cache=0&vercel-blob-delegation=different&vercel-blob-signature=signature`,
  ]) {
    const blob = {
      issueSignedToken: async () => ({
        clientSigningToken: "client-signing-token",
        delegationToken: "delegation",
        validUntil: Date.now() + 60_000,
      }),
      presignUrl: async () => ({ presignedUrl }),
    } as unknown as ArtifactBlobSdk;
    const store = new VercelPrivateBlobArtifactStore(config, blob);

    await expect(
      store.issueDownloadGrant({
        key,
        validUntil: new Date("2026-07-24T00:15:00.000Z"),
      }),
    ).rejects.toThrow("unsafe signed transfer URL");
  }
});

const verifiedKey = "v1/private/artifacts/art_abcdefghijklmnop/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

test("Vercel private Blob verification cancels an object that exceeds the caller's read bound", async () => {
  let canceled = false;
  const stream = new ReadableStream<Uint8Array>({
    cancel() { canceled = true; },
  });
  const blob = {
    get: async () => ({
      blob: { size: 4 * 1024 * 1024 + 1 },
      statusCode: 200,
      stream,
    }),
  } as unknown as ArtifactBlobSdk;
  const store = new VercelPrivateBlobArtifactStore(config, blob);

  await expect(store.verifySha256(verifiedKey, "a".repeat(64), 4 * 1024 * 1024)).rejects.toThrow("exceeds the verification limit");
  expect(canceled).toBe(true);
});

test("Vercel private Blob verification digests a multi-chunk stream at constant memory", async () => {
  const chunkCount = 96;
  const chunk = new Uint8Array(1024).fill(7);
  const expected = createHash("sha256");
  for (let index = 0; index < chunkCount; index += 1) expected.update(chunk);
  const totalBytes = chunkCount * chunk.byteLength;

  const newStream = () => {
    let emitted = 0;
    return new ReadableStream<Uint8Array>({
      pull(controller) {
        if (emitted === chunkCount) {
          controller.close();
          return;
        }
        emitted += 1;
        controller.enqueue(chunk);
      },
    });
  };
  const blobFor = (stream: ReadableStream<Uint8Array>) => ({
    get: async () => ({ blob: { size: totalBytes }, statusCode: 200, stream }),
  }) as unknown as ArtifactBlobSdk;

  const matching = new VercelPrivateBlobArtifactStore(config, blobFor(newStream()));
  expect(await matching.verifySha256(verifiedKey, expected.digest("hex"), totalBytes)).toBe(true);

  const mismatched = new VercelPrivateBlobArtifactStore(config, blobFor(newStream()));
  expect(await mismatched.verifySha256(verifiedKey, "b".repeat(64), totalBytes)).toBe(false);
});

test("Vercel private Blob verification cancels a stream that grows past the plan", async () => {
  let canceled = false;
  const chunk = new Uint8Array(1024);
  const stream = new ReadableStream<Uint8Array>({
    pull(controller) { controller.enqueue(chunk); },
    cancel() { canceled = true; },
  });
  const blob = {
    // A HEAD-consistent size that the body then exceeds must still be bounded.
    get: async () => ({ blob: { size: 2048 }, statusCode: 200, stream }),
  } as unknown as ArtifactBlobSdk;
  const store = new VercelPrivateBlobArtifactStore(config, blob);

  await expect(store.verifySha256(verifiedKey, "a".repeat(64), 2048)).rejects.toThrow("exceeds the verification limit");
  expect(canceled).toBe(true);
});
