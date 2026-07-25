import { getArtifactService } from "@/features/artifacts/factory";
import { DrizzleInboundReplayRepository } from "@/platform/database/inbound-email-replay-repository";
import { requireServiceToken } from "@/shared/auth/bearer";
import { methodNotAllowedResponse, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
export const runtime = "nodejs"; export const dynamic = "force-dynamic";
export async function GET(request: Request): Promise<Response> { try { await requireServiceToken(request, "cron"); const artifacts = await getArtifactService().reconcile(); await new DrizzleInboundReplayRepository().cleanup(new Date()); return jsonResponse(request, artifacts); } catch (error) { return problemResponse(error, request); } }
function unsupported(request: Request): Response { return methodNotAllowedResponse(request, ["GET"]); }
export { unsupported as DELETE, unsupported as HEAD, unsupported as OPTIONS, unsupported as PATCH, unsupported as POST, unsupported as PUT };
