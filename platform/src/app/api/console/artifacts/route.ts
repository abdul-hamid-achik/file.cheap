import {
  consoleArtifactListQuerySchema,
  consoleArtifactListResponseSchema,
} from "@/features/console/catalog/contracts";
import { getConsoleCatalogService } from "@/features/console/catalog/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const url = new URL(request.url);
    const query = parseRequest(consoleArtifactListQuerySchema, {
      cursor: url.searchParams.get("cursor") ?? undefined,
      direction: url.searchParams.get("direction") ?? undefined,
      kind: url.searchParams.get("kind") ?? undefined,
      limit: url.searchParams.has("limit") ? Number(url.searchParams.get("limit")) : undefined,
      producer: url.searchParams.get("producer") ?? undefined,
      q: url.searchParams.get("q") ?? undefined,
    });
    const page = await getConsoleCatalogService().listArtifacts(query, principal.userId);
    return jsonResponse(request, consoleArtifactListResponseSchema.parse(page));
  } catch (error) {
    return problemResponse(error, request);
  }
}
