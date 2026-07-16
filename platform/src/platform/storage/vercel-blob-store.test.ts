import { describe, expect, test } from "bun:test";
import {
  BlobNotFoundError,
  BlobPreconditionFailedError,
  BlobRequestAbortedError,
  BlobServiceNotAvailable,
  BlobServiceRateLimited,
} from "@vercel/blob";

import {
  type BlobSdk,
  VercelBlobStore,
} from "@/platform/storage/vercel-blob-store";
import type { PlatformConfig } from "@/shared/config/env";
import {
  CatalogPreconditionError,
  PlatformError,
} from "@/shared/errors/platform-error";

const config: PlatformConfig = {
  apiToken: "local-development-token",
  blobReadWriteToken: "test-blob-token",
  dataDirectory: "/unused",
  publicUrl: "http://127.0.0.1:3100",
  signingSecret: "test-signing-secret-that-is-long-enough",
  storageDriver: "vercel-blob",
};

describe("VercelBlobStore signed grants", () => {
  test("scopes an upload to one exact immutable pathname and size", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const blob = grantOnlyBlobSdk(calls);
    const store = new VercelBlobStore(config, blob);
    expect(store.verification).toBe("presence-size-etag");
    const key = `v1/vaults/test/objects/${"a".repeat(64)}.age`;

    const grant = await store.issueUploadGrant({
      contentType: "application/octet-stream",
      key,
      sha256: "a".repeat(64),
      sizeBytes: 1024,
      validUntil: new Date("2026-07-15T23:00:00.000Z"),
    });

    expect(grant).toMatchObject({
      headers: { "content-type": "application/octet-stream" },
      method: "PUT",
      url: "https://example.private.blob.vercel-storage.com/signed",
    });
    expect(calls[0]).toMatchObject({
      allowedContentTypes: ["application/octet-stream"],
      maximumSizeInBytes: 1024,
      operations: ["put"],
      pathname: key,
    });
    expect(calls[1]).toMatchObject({
      addRandomSuffix: false,
      allowOverwrite: false,
      maximumSizeInBytes: 1024,
      operation: "put",
      pathname: key,
    });
    expect(calls.flatMap(Object.values)).not.toContain("*");
  });

  test("scopes a download to one exact GET pathname", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const store = new VercelBlobStore(config, grantOnlyBlobSdk(calls));
    const key = `v1/vaults/test/objects/${"b".repeat(64)}.age`;

    const grant = await store.issueDownloadGrant({
      key,
      validUntil: new Date("2026-07-15T23:00:00.000Z"),
    });

    expect(grant.method).toBe("GET");
    expect(calls[0]).toMatchObject({ operations: ["get"], pathname: key });
    expect(calls[1]).toMatchObject({
      operation: "get",
      pathname: key,
      useCache: false,
    });
    expect(calls.flatMap(Object.values)).not.toContain("*");
  });

  test("inspects exact object metadata and maps a missing blob to null", async () => {
    const key = `v1/vaults/test/objects/${"c".repeat(64)}.age`;
    const presentBlob = {
      ...grantOnlyBlobSdk([]),
      head: async () => ({
        cacheControl: "public, max-age=0",
        contentDisposition: "attachment",
        contentType: "application/octet-stream",
        downloadUrl: "https://example.private.blob.vercel-storage.com/download",
        etag: "blob-etag",
        pathname: key,
        size: 42,
        uploadedAt: new Date("2026-07-15T23:00:00.000Z"),
        url: "https://example.private.blob.vercel-storage.com/object",
      }),
    } as unknown as BlobSdk;

    await expect(
      new VercelBlobStore(config, presentBlob).inspect(key),
    ).resolves.toMatchObject({
      contentType: "application/octet-stream",
      etag: "blob-etag",
      key,
      sizeBytes: 42,
      uploadedAt: "2026-07-15T23:00:00.000Z",
    });

    const missingBlob = {
      ...grantOnlyBlobSdk([]),
      head: async () => {
        throw new BlobNotFoundError();
      },
    } as unknown as BlobSdk;
    await expect(
      new VercelBlobStore(config, missingBlob).inspect(key),
    ).resolves.toBeNull();
  });

  test("reads catalog text without cache and preserves the Blob ETag", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const key = "v1/workspaces/default/catalog/v1.json";
    const blob = {
      ...grantOnlyBlobSdk([]),
      get: async (_key: string, options: Record<string, unknown>) => {
        calls.push(options);
        return {
          blob: { etag: "catalog-etag" },
          statusCode: 200,
          stream: new Blob(["{\"revision\":1}"]).stream(),
        };
      },
    } as unknown as BlobSdk;

    await expect(
      new VercelBlobStore(config, blob).readText(key),
    ).resolves.toEqual({
      body: '{"revision":1}',
      etag: "catalog-etag",
    });
    expect(calls[0]).toMatchObject({ access: "private", useCache: false });
  });

  test("uses ETag compare-and-swap and maps stale preconditions", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const key = "v1/workspaces/default/catalog/v1.json";
    const blob = {
      ...grantOnlyBlobSdk([]),
      put: async (
        _key: string,
        _body: string,
        options: Record<string, unknown>,
      ) => {
        calls.push(options);
        return { etag: `etag-${calls.length}` };
      },
    } as unknown as BlobSdk;
    const store = new VercelBlobStore(config, blob);

    await expect(store.writeText({ body: "first", key })).resolves.toEqual({
      etag: "etag-1",
    });
    await expect(
      store.writeText({ body: "second", expectedEtag: "etag-1", key }),
    ).resolves.toEqual({ etag: "etag-2" });
    expect(calls[0]).toMatchObject({
      access: "private",
      allowOverwrite: false,
      contentType: "application/json",
    });
    expect(calls[1]).toMatchObject({
      allowOverwrite: true,
      ifMatch: "etag-1",
    });

    const staleBlob = {
      ...grantOnlyBlobSdk([]),
      put: async () => {
        throw new BlobPreconditionFailedError();
      },
    } as unknown as BlobSdk;
    await expect(
      new VercelBlobStore(config, staleBlob).writeText({
        body: "stale",
        expectedEtag: "old-etag",
        key,
      }),
    ).rejects.toBeInstanceOf(CatalogPreconditionError);
  });

  test("maps a concurrent first catalog creation to a retryable precondition", async () => {
    const key = "v1/workspaces/default/catalog/v1.json";
    const blob = {
      ...grantOnlyBlobSdk([]),
      put: async () => {
        throw new BlobPreconditionFailedError();
      },
    } as unknown as BlobSdk;

    await expect(
      new VercelBlobStore(config, blob).writeText({ body: "first", key }),
    ).rejects.toBeInstanceOf(CatalogPreconditionError);

    const accessFailure = new Error("blob access denied");
    const unavailableBlob = {
      ...grantOnlyBlobSdk([]),
      put: async () => {
        throw accessFailure;
      },
    } as unknown as BlobSdk;
    await expect(
      new VercelBlobStore(config, unavailableBlob).writeText({ body: "first", key }),
    ).rejects.toBe(accessFailure);
  });

  test("propagates cancellation to Blob command options", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const controller = new AbortController();
    const key = `v1/vaults/test/objects/${"d".repeat(64)}.age`;
    const blob = {
      ...grantOnlyBlobSdk(calls),
      head: async (_key: string, options: Record<string, unknown>) => {
        calls.push(options);
        throw new BlobNotFoundError();
      },
    } as unknown as BlobSdk;
    const store = new VercelBlobStore(config, blob);

    await store.inspect(key, controller.signal);
    await store.issueDownloadGrant(
      { key, validUntil: new Date("2026-07-15T23:00:00.000Z") },
      controller.signal,
    );

    expect(calls[0].abortSignal).toBe(controller.signal);
    expect(calls[1].abortSignal).toBe(controller.signal);
  });

  test("normalizes retryable and canceled Blob failures", async () => {
    const key = "v1/workspaces/default/catalog/v1.json";
    for (const [failure, expected] of [
      [
        new BlobServiceRateLimited(7),
        { code: "storage_rate_limited", retryAfterSeconds: 7, status: 503 },
      ],
      [
        new BlobServiceNotAvailable(),
        { code: "storage_unavailable", retryAfterSeconds: 1, status: 503 },
      ],
      [
        new BlobRequestAbortedError(),
        { code: "request_aborted", status: 408 },
      ],
    ] as const) {
      const blob = {
        ...grantOnlyBlobSdk([]),
        put: async () => {
          throw failure;
        },
      } as unknown as BlobSdk;

      try {
        await new VercelBlobStore(config, blob).writeText({ body: "{}", key });
        throw new Error("Expected Blob failure to reject");
      } catch (error) {
        expect(error).toBeInstanceOf(PlatformError);
        expect(error).toMatchObject(expected);
      }
    }
  });

  test("normalizes raw fetch aborts while reading a Blob catalog", async () => {
    const key = "v1/workspaces/default/catalog/v1.json";
    for (const get of [
      async () => {
        throw new DOMException("The request was aborted", "AbortError");
      },
      async () => ({
        blob: { etag: "catalog-etag" },
        statusCode: 200,
        stream: new ReadableStream<Uint8Array>({
          start(controller) {
            controller.error(
              new DOMException("The stream was aborted", "AbortError"),
            );
          },
        }),
      }),
    ]) {
      const blob = {
        ...grantOnlyBlobSdk([]),
        get,
      } as unknown as BlobSdk;

      await expect(
        new VercelBlobStore(config, blob).readText(key),
      ).rejects.toMatchObject({ code: "request_aborted", status: 408 });
    }
  });
});

function grantOnlyBlobSdk(calls: Array<Record<string, unknown>>): BlobSdk {
  return {
    get: async () => null,
    head: async () => {
      throw new Error("not used");
    },
    issueSignedToken: async (options) => {
      calls.push(options as Record<string, unknown>);
      return {
        clientSigningToken: "client-signing-token",
        delegationToken: "delegation-token",
        validUntil: options.validUntil ?? Date.now() + 60_000,
      };
    },
    presignUrl: async (_token, options) => {
      calls.push(options as unknown as Record<string, unknown>);
      return {
        presignedUrl: "https://example.private.blob.vercel-storage.com/signed",
      };
    },
    put: async () => {
      throw new Error("not used");
    },
  } as BlobSdk;
}
