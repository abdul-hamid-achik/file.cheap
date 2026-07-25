import { randomUUID } from "node:crypto";

import { assertSafeArtifactObjectKey, type ArtifactObjectMetadata, type ArtifactObjectStore, type ArtifactTransferGrant } from "@/platform/artifacts/object-store";

type Stored = ArtifactObjectMetadata & { bytes?: Uint8Array };

/** Test adapter only. Route tests seed completed direct uploads explicitly. */
export class InMemoryArtifactObjectStore implements ArtifactObjectStore {
  readonly driver = "memory";
  private readonly objects = new Map<string, Stored>();

  async delete(key: string): Promise<void> { this.objects.delete(key); }
  async inspect(key: string): Promise<ArtifactObjectMetadata | null> { return this.objects.get(key) ?? null; }
  async readBytes(key: string): Promise<Uint8Array | null> { return this.objects.get(key)?.bytes ?? null; }
  async issueDownloadGrant(input: { key: string; validUntil: Date }): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    return { expiresAt: input.validUntil.toISOString(), headers: {}, method: "GET", url: `https://artifact.test/download/${input.key}` };
  }
  async issueUploadGrant(input: { contentType: string; key: string; sizeBytes: number; validUntil: Date }): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    return { expiresAt: input.validUntil.toISOString(), headers: { "content-type": input.contentType }, method: "PUT", url: `https://artifact.test/upload/${input.key}` };
  }
  seed(input: Omit<ArtifactObjectMetadata, "etag" | "uploadedAt"> & Partial<Pick<ArtifactObjectMetadata, "etag" | "uploadedAt">> & { bytes?: Uint8Array }): void {
    this.objects.set(input.key, { ...input, etag: input.etag ?? randomUUID(), uploadedAt: input.uploadedAt ?? new Date().toISOString() });
  }
}
