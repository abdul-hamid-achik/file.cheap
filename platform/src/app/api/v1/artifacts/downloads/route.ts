import { artifactDownloadInputSchema, artifactDownloadResponseSchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { readPolicyFor, requireServiceToken } from "@/shared/auth/bearer";
import { methodNotAllowedResponse, parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
export const runtime = "nodejs"; export const dynamic = "force-dynamic";
export async function POST(request: Request): Promise<Response> { try { const principal = await requireServiceToken(request, "read"); return jsonResponse(request, artifactDownloadResponseSchema.parse(await getArtifactService().download(parseRequest(artifactDownloadInputSchema, await parseJson(request)), request.signal, readPolicyFor(principal))), { status: 201 }); } catch (error) { return problemResponse(error, request); } }
function unsupported(request: Request): Response { return methodNotAllowedResponse(request, ["POST"]); }
export { unsupported as DELETE, unsupported as GET, unsupported as HEAD, unsupported as OPTIONS, unsupported as PATCH, unsupported as PUT };
