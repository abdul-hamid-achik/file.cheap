export type ArtifactObjectMetadata = {
  contentType: string;
  etag: string;
  key: string;
  sizeBytes: number;
  uploadedAt: string;
};

export type ArtifactTransferGrant = {
  expiresAt: string;
  headers: Record<string, string>;
  method: "GET" | "PUT";
  url: string;
};

export interface ArtifactObjectStore {
  readonly driver: string;
  delete(key: string, signal?: AbortSignal): Promise<void>;
  inspect(key: string, signal?: AbortSignal): Promise<ArtifactObjectMetadata | null>;
  readBytes(key: string, signal?: AbortSignal): Promise<Uint8Array | null>;
  issueDownloadGrant(input: { key: string; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant>;
  issueUploadGrant(input: { contentType: string; key: string; sizeBytes: number; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant>;
}

const keyPattern = /^[a-z0-9][a-z0-9._/-]*$/;

export function assertSafeArtifactObjectKey(key: string): void {
  const parts = key.split("/");
  if (!keyPattern.test(key) || key.includes("..") || key.startsWith("/") || key.endsWith("/") || parts.some((part) => !part || part === "." || part === "..")) {
    throw new Error("Unsafe artifact object key");
  }
}
