import {
  consoleRunListQuerySchema,
  consoleRunListResponseSchema,
} from "@/features/console/catalog/contracts";
import { getConsoleCatalogService } from "@/features/console/catalog/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const params = new URL(request.url).searchParams;
    const query = parseRequest(consoleRunListQuerySchema, {
      cursor: params.get("cursor") ?? undefined,
      direction: params.get("direction") ?? undefined,
      from: params.get("from") ?? undefined,
      health: params.get("health") ?? undefined,
      limit: params.has("limit") ? Number(params.get("limit")) : undefined,
      producer: params.get("producer") ?? undefined,
      q: params.get("q") ?? undefined,
      status: params.get("status") ?? undefined,
      to: params.get("to") ?? undefined,
    });
    const page = await getConsoleCatalogService().listRuns(query, principal.userId);
    return jsonResponse(request, consoleRunListResponseSchema.parse(page));
  } catch (error) {
    return problemResponse(error, request);
  }
}
