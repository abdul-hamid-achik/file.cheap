import type {
  ConsoleArtifactListQuery,
  ConsoleArtifactListResponse,
  ConsoleRunListQuery,
  ConsoleRunListResponse,
} from "@/features/console/catalog/contracts";
import type { ConsoleCatalogRepository } from "@/features/console/catalog/repository";

export class ConsoleCatalogService {
  constructor(
    private readonly repository: ConsoleCatalogRepository,
    private readonly now: () => Date = () => new Date(),
  ) {}

  async listArtifacts(
    query: ConsoleArtifactListQuery,
    ownerAccountId: string,
  ): Promise<ConsoleArtifactListResponse> {
    return {
      ...await this.repository.listArtifacts(query, ownerAccountId, this.now()),
      version: "filecheap-console-artifacts/1",
    };
  }

  async listRuns(
    query: ConsoleRunListQuery,
    ownerAccountId: string,
  ): Promise<ConsoleRunListResponse> {
    return {
      ...await this.repository.listRuns(query, ownerAccountId, this.now()),
      version: "filecheap-console-runs/1",
    };
  }
}
