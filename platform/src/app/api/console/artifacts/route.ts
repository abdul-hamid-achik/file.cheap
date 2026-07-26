import { artifactListQuerySchema, artifactListResponseSchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const url = new URL(request.url);
    const query = parseRequest(artifactListQuerySchema, {
      after: url.searchParams.get("after") ?? undefined,
      limit: url.searchParams.has("limit") ? Number(url.searchParams.get("limit")) : undefined,
    });
    const page = await getArtifactService().list(query, principal.userId);
    return jsonResponse(request, artifactListResponseSchema.parse({ ...page, version: "filecheap-artifacts/1" }));
  } catch (error) {
    return problemResponse(error, request);
  }
}
