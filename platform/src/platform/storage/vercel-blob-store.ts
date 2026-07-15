import {
  BlobNotFoundError,
  BlobPreconditionFailedError,
  get,
  head,
  issueSignedToken,
  presignUrl,
  put,
} from "@vercel/blob";

import { getConfig, type PlatformConfig } from "@/shared/config/env";
import { CatalogPreconditionError } from "@/shared/errors/platform-error";
import {
  assertSafeObjectKey,
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

  constructor(
    private readonly config: PlatformConfig = getConfig(),
    private readonly blob: BlobSdk = defaultBlobSdk,
  ) {}

  async inspect(key: string): Promise<ObjectMetadata | null> {
    assertSafeObjectKey(key);
    try {
      const blob = await this.blob.head(key, this.commandOptions());
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
      throw error;
    }
  }

  async issueUploadGrant(input: {
    contentType: string;
    key: string;
    sha256: string;
    sizeBytes: number;
    validUntil: Date;
  }): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const validUntil = input.validUntil.getTime();
    const signedToken = await this.blob.issueSignedToken({
      ...this.commandOptions(),
      allowedContentTypes: [input.contentType],
      maximumSizeInBytes: input.sizeBytes,
      operations: ["put"],
      pathname: input.key,
      validUntil,
    });
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

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: { "content-type": input.contentType },
      method: "PUT",
      url: presignedUrl,
    };
  }

  async issueDownloadGrant(input: {
    key: string;
    validUntil: Date;
  }): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const validUntil = input.validUntil.getTime();
    const signedToken = await this.blob.issueSignedToken({
      ...this.commandOptions(),
      operations: ["get"],
      pathname: input.key,
      validUntil,
    });
    const { presignedUrl } = await this.blob.presignUrl(signedToken, {
      access: "private",
      operation: "get",
      pathname: input.key,
      validUntil,
    });

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: {},
      method: "GET",
      url: presignedUrl,
    };
  }

  async readText(key: string): Promise<TextObject | null> {
    assertSafeObjectKey(key);
    const result = await this.blob.get(key, {
      access: "private",
      ...this.commandOptions(),
      useCache: false,
    });
    if (!result) {
      return null;
    }
    if (result.statusCode !== 200) {
      throw new Error(`Unexpected Blob response: ${result.statusCode}`);
    }

    return {
      body: await new Response(result.stream).text(),
      etag: result.blob.etag,
    };
  }

  async writeText(input: {
    body: string;
    expectedEtag?: string;
    key: string;
  }): Promise<{ etag: string }> {
    assertSafeObjectKey(input.key);
    try {
      const result = await this.blob.put(input.key, input.body, {
        access: "private",
        addRandomSuffix: false,
        allowOverwrite: Boolean(input.expectedEtag),
        contentType: "application/json",
        ifMatch: input.expectedEtag,
        ...this.commandOptions(),
      });
      return { etag: result.etag };
    } catch (error) {
      if (error instanceof BlobPreconditionFailedError) {
        throw new CatalogPreconditionError();
      }
      throw error;
    }
  }

  private commandOptions(): { token?: string } {
    return this.config.blobReadWriteToken
      ? { token: this.config.blobReadWriteToken }
      : {};
  }
}
