import { getPlanReceiptKeyring } from "@/features/artifacts/plan-receipt-config";
import { ArtifactService } from "@/features/artifacts/service";
import { getConfig } from "@/shared/config/env";
import { getArtifactObjectStore } from "@/platform/artifacts/factory";
import { DrizzleArtifactRepository, type ArtifactRepository } from "@/platform/database/repository";

let service: ArtifactService | undefined;

export function getArtifactService(): ArtifactService {
  service ??= new ArtifactService(
    getArtifactObjectStore(),
    new DrizzleArtifactRepository(),
    getPlanReceiptKeyring(),
    undefined,
    getConfig().publicUrl,
  );
  return service;
}

export function setArtifactServiceForTests(value?: ArtifactService): void { service = value; }
export type { ArtifactRepository };
