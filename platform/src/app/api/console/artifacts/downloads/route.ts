import { artifactDownloadInputSchema, artifactDownloadResponseSchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

export async function POST(request: Request): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const principal = await requireConsolePrincipal(request);
    const input = parseRequest(artifactDownloadInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({
      action: "artifact-download",
      key: `${principal.userId}\n${input.artifactId}`,
      limit: 30,
      request,
      windowSeconds: 15 * 60,
    });
    const value = await getArtifactService().download(input, request.signal, undefined, principal.userId);
    return jsonResponse(request, artifactDownloadResponseSchema.parse(value), { status: 201 });
  } catch (error) {
    return problemResponse(error, request);
  }
}
