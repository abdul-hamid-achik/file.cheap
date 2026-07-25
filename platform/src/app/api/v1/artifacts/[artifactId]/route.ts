import { artifactIdSchema, artifactSummarySchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { requireServiceToken } from "@/shared/auth/bearer";
import { methodNotAllowedResponse, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
export const runtime = "nodejs"; export const dynamic = "force-dynamic";
export async function GET(request: Request, context: { params: Promise<{ artifactId: string }> }): Promise<Response> { try { await requireServiceToken(request, "admin"); const { artifactId } = await context.params; return jsonResponse(request, artifactSummarySchema.parse(await getArtifactService().get(artifactIdSchema.parse(artifactId)))); } catch (error) { return problemResponse(error, request); } }
function unsupported(request: Request): Response { return methodNotAllowedResponse(request, ["GET", "HEAD"]); }
export { unsupported as DELETE, unsupported as OPTIONS, unsupported as PATCH, unsupported as POST, unsupported as PUT };
