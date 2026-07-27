import { createHash } from "node:crypto";

import {
  BlobNotFoundError,
  BlobRequestAbortedError,
  BlobServiceNotAvailable,
  BlobServiceRateLimited,
  del,
  get,
  head,
  issueSignedToken,
  presignUrl,
} from "@vercel/blob";

import { getConfig, type PlatformConfig } from "@/shared/config/env";
import { PlatformError } from "@/shared/errors/platform-error";
import {
  assertSafeArtifactObjectKey,
  type ArtifactObjectMetadata,
  type ArtifactObjectStore,
  type ArtifactTransferGrant,
} from "@/platform/artifacts/object-store";

export type ArtifactBlobSdk = {
  del: typeof del;
  get: typeof get;
  head: typeof head;
  issueSignedToken: typeof issueSignedToken;
  presignUrl: typeof presignUrl;
};

const defaultBlobSdk: ArtifactBlobSdk = { del, get, head, issueSignedToken, presignUrl };

export class VercelPrivateBlobArtifactStore implements ArtifactObjectStore {
  readonly driver = "vercel-private-blob";

  constructor(
    private readonly config: PlatformConfig = getConfig(),
    private readonly blob: ArtifactBlobSdk = defaultBlobSdk,
  ) {}

  async delete(key: string, signal?: AbortSignal): Promise<void> {
    assertSafeArtifactObjectKey(key);
    try {
      await this.blob.del(key, this.options(signal));
    } catch (error) {
      if (error instanceof BlobNotFoundError) return;
      throw normalizeBlobError(error, signal);
    }
  }

  async inspect(key: string, signal?: AbortSignal): Promise<ArtifactObjectMetadata | null> {
    assertSafeArtifactObjectKey(key);
    try {
      const blob = await this.blob.head(key, this.options(signal));
      return { contentType: blob.contentType, etag: blob.etag, key: blob.pathname, sizeBytes: blob.size, uploadedAt: blob.uploadedAt.toISOString() };
    } catch (error) {
      if (error instanceof BlobNotFoundError) return null;
      throw normalizeBlobError(error, signal);
    }
  }

  // Verification digests the private object as it arrives. Memory stays O(1),
  // so the accepted artifact size is bounded by the caller's plan and the
  // platform ceiling rather than by the function's heap.
  async verifySha256(key: string, expectedSha256: string, maxBytes: number, signal?: AbortSignal): Promise<boolean> {
    assertSafeArtifactObjectKey(key);
    try {
      const result = await this.blob.get(key, { access: "private", ...this.options(signal), useCache: false });
      if (!result) return false;
      if (result.statusCode !== 200) throw new Error("Unexpected private Blob response");
      if (result.blob.size > maxBytes) {
        await result.stream.cancel();
        throw new Error("Private artifact exceeds the verification limit");
      }
      return await streamSha256Matches(result.stream, expectedSha256, maxBytes);
    } catch (error) {
      if (error instanceof BlobNotFoundError) return false;
      throw normalizeBlobError(error, signal);
    }
  }

  async issueUploadGrant(input: { contentType: string; key: string; sizeBytes: number; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    try {
      const token = await this.blob.issueSignedToken({ ...this.options(signal), allowedContentTypes: [input.contentType], maximumSizeInBytes: input.sizeBytes, operations: ["put"], pathname: input.key, validUntil: input.validUntil.getTime() });
      const { presignedUrl } = await this.blob.presignUrl(token, { access: "private", addRandomSuffix: false, allowedContentTypes: [input.contentType], allowOverwrite: false, maximumSizeInBytes: input.sizeBytes, operation: "put", pathname: input.key, validUntil: input.validUntil.getTime() });
      return { expiresAt: input.validUntil.toISOString(), headers: { "content-type": input.contentType }, method: "PUT", url: requireExactPresignedUrl(presignedUrl, input.key, "put", token.delegationToken) };
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
  }

  async issueDownloadGrant(input: { key: string; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    try {
      const token = await this.blob.issueSignedToken({ ...this.options(signal), operations: ["get"], pathname: input.key, validUntil: input.validUntil.getTime() });
      const { presignedUrl } = await this.blob.presignUrl(token, { access: "private", operation: "get", pathname: input.key, useCache: false, validUntil: input.validUntil.getTime() });
      return { expiresAt: input.validUntil.toISOString(), headers: {}, method: "GET", url: requireExactPresignedUrl(presignedUrl, input.key, "get", token.delegationToken) };
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
  }

  private options(signal?: AbortSignal): { abortSignal?: AbortSignal; token?: string } {
    return { ...(signal ? { abortSignal: signal } : {}), ...(this.config.blobReadWriteToken ? { token: this.config.blobReadWriteToken } : {}) };
  }
}

function requireExactPresignedUrl(
  value: string,
  key: string,
  operation: "get" | "put",
  delegationToken: string,
): string {
  if (value.length > 16_384) {
    throw new BlobGrantShapeError("length");
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new BlobGrantShapeError("unparseable");
  }
  const exactPath =
    operation === "get"
      ? url.pathname === `/${key}`
      : url.origin === "https://vercel.com" &&
        url.pathname === "/api/blob/" &&
        url.searchParams.getAll("pathname").length === 1 &&
        url.searchParams.get("pathname") === key;
  const exactHost =
    operation === "get"
      ? /^[A-Za-z0-9_][A-Za-z0-9_-]{0,127}\.private\.blob\.vercel-storage\.com$/u.test(
          url.hostname,
        )
      : url.hostname === "vercel.com";
  const allowedQueryKeys =
    operation === "get"
      ? new Set([
          "cache",
          "vercel-blob-delegation",
          "vercel-blob-signature",
          "vercel-blob-valid-until",
        ])
      : new Set([
          "pathname",
          "vercel-blob-add-random-suffix",
          "vercel-blob-allow-overwrite",
          "vercel-blob-allowed-content-types",
          "vercel-blob-cache-control-max-age",
          "vercel-blob-maximum-size-in-bytes",
          "vercel-blob-signature",
          "vercel-blob-delegation",
          "vercel-blob-valid-until",
        ]);
  const exactQuery =
    [...url.searchParams.keys()].every((name) =>
      allowedQueryKeys.has(name),
    ) &&
    [...allowedQueryKeys].every(
      (name) => url.searchParams.getAll(name).length <= 1,
    ) &&
    (operation !== "get" ||
      (
        url.searchParams.getAll("cache").length === 1 &&
        url.searchParams.get("cache") === "0"
      ));
  if (
    url.protocol !== "https:" ||
    url.port ||
    url.username ||
    url.password ||
    url.hash ||
    !exactHost ||
    !exactPath ||
    !exactQuery ||
    url.searchParams.getAll("vercel-blob-delegation").length !== 1 ||
    url.searchParams.get("vercel-blob-delegation") !== delegationToken ||
    url.searchParams.getAll("vercel-blob-signature").length !== 1 ||
    !url.searchParams.get("vercel-blob-signature")
  ) {
    // Which of these failed is the whole diagnosis, and all three throw sites
    // in this function used to raise the same bare `Error` with the same
    // sentence. `problem.ts` keeps only `error.name` — deliberately, because
    // this URL carries a delegation token and a signature — so production
    // logged `errorName: "Error"` and nothing else, and a 500 here could not
    // be told apart from any other unhandled throw.
    //
    // The unexpected query *keys* are safe to name: they are Vercel's own
    // parameter names, and it is the values that are secret. If Vercel adds a
    // parameter to its presigned URLs, this allowlist goes stale and every
    // upload grant fails — that is the failure this makes legible in one
    // request instead of none.
    const unexpected = [...url.searchParams.keys()]
      .filter((name) => !allowedQueryKeys.has(name))
      .slice(0, 8);
    throw new BlobGrantShapeError(
      !exactHost
        ? "host"
        : !exactPath
          ? "path"
          : unexpected.length > 0
            ? "query-unexpected"
            : !exactQuery
              ? "query-shape"
              : "signature",
      unexpected,
    );
  }
  return url.toString();
}

/**
 * A presigned URL that is not exactly the one that was asked for. Carries a
 * reason and, when the mismatch is an unrecognized query parameter, its name —
 * never the URL, the delegation token, or the signature.
 */
export class BlobGrantShapeError extends Error {
  readonly reason: string;
  readonly unexpectedQueryKeys: readonly string[];

  constructor(reason: string, unexpectedQueryKeys: readonly string[] = []) {
    super("Vercel Blob returned an unsafe signed transfer URL");
    this.name = "BlobGrantShapeError";
    this.reason = reason;
    this.unexpectedQueryKeys = Object.freeze([...unexpectedQueryKeys]);
  }
}

function normalizeBlobError(error: unknown, signal?: AbortSignal): unknown {
  if (error instanceof BlobRequestAbortedError || (error instanceof Error && error.name === "AbortError") || signal?.aborted) {
    return new PlatformError({ code: "request_aborted", detail: "The private storage request was canceled.", status: 408, title: "Storage request canceled" });
  }
  if (error instanceof BlobServiceRateLimited || error instanceof BlobServiceNotAvailable) {
    return new PlatformError({ code: "storage_unavailable", detail: "Private artifact storage is temporarily unavailable. Retry this operation.", retryAfterSeconds: error instanceof BlobServiceRateLimited ? error.retryAfter || 1 : 1, status: 503, title: "Storage unavailable" });
  }
  return error;
}

async function streamSha256Matches(
  stream: ReadableStream<Uint8Array>,
  expectedSha256: string,
  limit: number,
): Promise<boolean> {
  const reader = stream.getReader();
  const digest = createHash("sha256");
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > limit) {
        await reader.cancel();
        throw new Error("Private artifact exceeds the verification limit");
      }
      digest.update(value);
    }
  } finally {
    reader.releaseLock();
  }
  return digest.digest("hex") === expectedSha256;
}
