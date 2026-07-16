import {
  BlobNotFoundError,
  BlobPreconditionFailedError,
  BlobRequestAbortedError,
  BlobServiceNotAvailable,
  BlobServiceRateLimited,
  get,
  head,
  issueSignedToken,
  presignUrl,
  put,
} from "@vercel/blob";

import { getConfig, type PlatformConfig } from "@/shared/config/env";
import {
  CatalogPreconditionError,
  PlatformError,
} from "@/shared/errors/platform-error";
import {
  assertSafeObjectKey,
  throwIfStorageOperationAborted,
  type ObjectMetadata,
  type ObjectStore,
  type TextObject,
  type TransferGrant,
} from "@/platform/storage/object-store";

export type BlobSdk = {
  get: typeof get;
  head: typeof head;
  issueSignedToken: typeof issueSignedToken;
  presignUrl: typeof presignUrl;
  put: typeof put;
};

const defaultBlobSdk: BlobSdk = {
  get,
  head,
  issueSignedToken,
  presignUrl,
  put,
};

export class VercelBlobStore implements ObjectStore {
  readonly driver = "vercel-blob" as const;
  // Blob HEAD proves presence, size, and an opaque ETag. It does not prove the
  // caller-declared SHA-256, so clients must perform a full recovery check.
  readonly verification = "presence-size-etag" as const;

  constructor(
    private readonly config: PlatformConfig = getConfig(),
    private readonly blob: BlobSdk = defaultBlobSdk,
  ) {}

  async inspect(
    key: string,
    signal?: AbortSignal,
  ): Promise<ObjectMetadata | null> {
    assertSafeObjectKey(key);
    try {
      const blob = await this.blob.head(key, this.commandOptions(signal));
      return {
        contentType: blob.contentType,
        etag: blob.etag,
        key: blob.pathname,
        sizeBytes: blob.size,
        uploadedAt: blob.uploadedAt.toISOString(),
      };
    } catch (error) {
      if (error instanceof BlobNotFoundError) {
        return null;
      }
      throw normalizeBlobError(error, signal);
    }
  }

  async issueUploadGrant(
    input: {
      contentType: string;
      key: string;
      sha256: string;
      sizeBytes: number;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const validUntil = input.validUntil.getTime();
    let signedToken: Awaited<ReturnType<BlobSdk["issueSignedToken"]>>;
    try {
      signedToken = await this.blob.issueSignedToken({
        ...this.commandOptions(signal),
        allowedContentTypes: [input.contentType],
        maximumSizeInBytes: input.sizeBytes,
        operations: ["put"],
        pathname: input.key,
        validUntil,
      });
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
    throwIfStorageOperationAborted(signal);
    const { presignedUrl } = await this.blob.presignUrl(signedToken, {
      access: "private",
      addRandomSuffix: false,
      allowedContentTypes: [input.contentType],
      allowOverwrite: false,
      maximumSizeInBytes: input.sizeBytes,
      operation: "put",
      pathname: input.key,
      validUntil,
    });
    throwIfStorageOperationAborted(signal);

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: { "content-type": input.contentType },
      method: "PUT",
      url: presignedUrl,
    };
  }

  async issueDownloadGrant(
    input: {
      key: string;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const validUntil = input.validUntil.getTime();
    let signedToken: Awaited<ReturnType<BlobSdk["issueSignedToken"]>>;
    try {
      signedToken = await this.blob.issueSignedToken({
        ...this.commandOptions(signal),
        operations: ["get"],
        pathname: input.key,
        validUntil,
      });
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
    throwIfStorageOperationAborted(signal);
    const { presignedUrl } = await this.blob.presignUrl(signedToken, {
      access: "private",
      operation: "get",
      pathname: input.key,
      useCache: false,
      validUntil,
    });
    throwIfStorageOperationAborted(signal);

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: {},
      method: "GET",
      url: presignedUrl,
    };
  }

  async readText(
    key: string,
    signal?: AbortSignal,
  ): Promise<TextObject | null> {
    assertSafeObjectKey(key);
    let result: Awaited<ReturnType<BlobSdk["get"]>>;
    try {
      result = await this.blob.get(key, {
        access: "private",
        ...this.commandOptions(signal),
        useCache: false,
      });
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
    if (!result) {
      return null;
    }
    if (result.statusCode !== 200) {
      throw new Error(`Unexpected Blob response: ${result.statusCode}`);
    }

    try {
      return {
        body: await new Response(result.stream).text(),
        etag: result.blob.etag,
      };
    } catch (error) {
      throw normalizeBlobError(error, signal);
    }
  }

  async writeText(
    input: {
      body: string;
      expectedEtag?: string;
      key: string;
    },
    signal?: AbortSignal,
  ): Promise<{ etag: string }> {
    assertSafeObjectKey(input.key);
    try {
      const result = await this.blob.put(input.key, input.body, {
        access: "private",
        addRandomSuffix: false,
        allowOverwrite: Boolean(input.expectedEtag),
        contentType: "application/json",
        ifMatch: input.expectedEtag,
        ...this.commandOptions(signal),
      });
      return { etag: result.etag };
    } catch (error) {
      if (error instanceof BlobPreconditionFailedError) {
        throw new CatalogPreconditionError();
      }
      throw normalizeBlobError(error, signal);
    }
  }

  private commandOptions(signal?: AbortSignal): {
    abortSignal?: AbortSignal;
    token?: string;
  } {
    return {
      ...(signal ? { abortSignal: signal } : {}),
      ...(this.config.blobReadWriteToken
        ? { token: this.config.blobReadWriteToken }
        : {}),
    };
  }
}

function normalizeBlobError(error: unknown, signal?: AbortSignal): unknown {
  if (
    error instanceof BlobRequestAbortedError ||
    (error instanceof Error && error.name === "AbortError") ||
    signal?.aborted
  ) {
    return new PlatformError({
      code: "request_aborted",
      detail: "The Blob request was canceled before it completed.",
      status: 408,
      title: "Storage request canceled",
    });
  }
  if (error instanceof BlobServiceRateLimited) {
    return new PlatformError({
      code: "storage_rate_limited",
      detail: "Vercel Blob is rate limiting this operation. Retry after the advertised delay.",
      retryAfterSeconds: error.retryAfter || 1,
      status: 503,
      title: "Storage rate limited",
    });
  }
  if (error instanceof BlobServiceNotAvailable) {
    return new PlatformError({
      code: "storage_unavailable",
      detail: "Vercel Blob is temporarily unavailable. Retry this operation.",
      retryAfterSeconds: 1,
      status: 503,
      title: "Storage unavailable",
    });
  }
  return error;
}
