import { VercelPrivateBlobArtifactStore } from "@/platform/artifacts/vercel-blob-store";
import type { ArtifactObjectStore } from "@/platform/artifacts/object-store";

let artifactObjectStore: ArtifactObjectStore | undefined;

export function getArtifactObjectStore(): ArtifactObjectStore {
  artifactObjectStore ??= new VercelPrivateBlobArtifactStore();
  return artifactObjectStore;
}

export function setArtifactObjectStoreForTests(store?: ArtifactObjectStore): void {
  artifactObjectStore = store;
}
