import { ArtifactService } from "@/features/artifacts/service";
import { getArtifactObjectStore } from "@/platform/artifacts/factory";
import { DrizzleArtifactRepository, type ArtifactRepository } from "@/platform/database/repository";

let service: ArtifactService | undefined;

export function getArtifactService(): ArtifactService {
  service ??= new ArtifactService(getArtifactObjectStore(), new DrizzleArtifactRepository());
  return service;
}

export function setArtifactServiceForTests(value?: ArtifactService): void { service = value; }
export type { ArtifactRepository };
