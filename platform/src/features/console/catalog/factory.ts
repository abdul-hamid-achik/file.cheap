import { ConsoleCatalogService } from "@/features/console/catalog/service";
import { DrizzleConsoleCatalogRepository } from "@/platform/database/console-catalog-repository";

let service: ConsoleCatalogService | undefined;

export function getConsoleCatalogService(): ConsoleCatalogService {
  service ??= new ConsoleCatalogService(new DrizzleConsoleCatalogRepository());
  return service;
}

export function setConsoleCatalogServiceForTests(
  value?: ConsoleCatalogService,
): void {
  service = value;
}
