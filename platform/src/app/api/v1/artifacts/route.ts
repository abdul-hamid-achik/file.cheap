import { artifactListQuerySchema, artifactListResponseSchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { requireServiceToken } from "@/shared/auth/bearer";
import { methodNotAllowedResponse, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
export const runtime = "nodejs"; export const dynamic = "force-dynamic";
export async function GET(request: Request): Promise<Response> { try { await requireServiceToken(request, "admin"); const url = new URL(request.url); const query = artifactListQuerySchema.parse({ after: url.searchParams.get("after") ?? undefined, limit: url.searchParams.has("limit") ? Number(url.searchParams.get("limit")) : undefined }); const page = await getArtifactService().list(query); return jsonResponse(request, artifactListResponseSchema.parse({ ...page, version: "filecheap-artifacts/1" })); } catch (error) { return problemResponse(error, request); } }
function unsupported(request: Request): Response { return methodNotAllowedResponse(request, ["GET", "HEAD"]); }
export { unsupported as DELETE, unsupported as OPTIONS, unsupported as PATCH, unsupported as POST, unsupported as PUT };
