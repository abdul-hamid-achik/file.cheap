import type {
  ConsoleArtifactListQuery,
  ConsoleArtifactListResponse,
  ConsoleRunListQuery,
  ConsoleRunListResponse,
} from "@/features/console/catalog/contracts";

export interface ConsoleCatalogRepository {
  listArtifacts(
    query: ConsoleArtifactListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleArtifactListResponse, "version">>;
  listRuns(
    query: ConsoleRunListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleRunListResponse, "version">>;
}
