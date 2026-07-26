import { runListQuerySchema, runListResponseSchema } from "@/features/runs/contracts";
import { getRunService } from "@/features/runs/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const params = new URL(request.url).searchParams;
    const query = parseRequest(runListQuerySchema, {
      after: params.get("after") ?? undefined,
      from: params.get("from") ?? undefined,
      health: params.get("health") ?? undefined,
      limit: params.has("limit") ? Number(params.get("limit")) : undefined,
      producer: params.get("producer") ?? undefined,
      q: params.get("q") ?? undefined,
      status: params.get("status") ?? undefined,
      to: params.get("to") ?? undefined,
    });
    const page = await getRunService().list(query, principal.userId);
    return jsonResponse(request, runListResponseSchema.parse({ ...page, version: "filecheap-runs/1" }));
  } catch (error) {
    return problemResponse(error, request);
  }
}
