import { artifactPlanInputSchema, artifactPlanResultSchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { assertProducerSizeQuota } from "@/features/artifacts/service";
import { requireAuthorizedArtifact, requireServiceToken } from "@/shared/auth/bearer";
import { methodNotAllowedResponse, parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { getConfig } from "@/shared/config/env";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request: Request): Promise<Response> {
  try {
    const principal = await requireServiceToken(request, "ingest");
    const input = parseRequest(artifactPlanInputSchema, await parseJson(request));
    requireAuthorizedArtifact(principal, input);
    assertProducerSizeQuota(input.sizeBytes, principal, input.kind);
    const result = artifactPlanResultSchema.parse(await getArtifactService().plan(input, request.signal, getConfig().ownerAccountId));
    return jsonResponse(request, result, { status: result.artifact.state === "committed" ? 200 : 201 });
  } catch (error) { return problemResponse(error, request); }
}

function unsupported(request: Request): Response { return methodNotAllowedResponse(request, ["POST"]); }
export { unsupported as DELETE, unsupported as GET, unsupported as HEAD, unsupported as OPTIONS, unsupported as PATCH, unsupported as PUT };
