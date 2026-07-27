import { artifactIdSchema, artifactSummarySchema } from "@/features/artifacts/contracts";
import { getArtifactService } from "@/features/artifacts/factory";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { requireConsolePrincipal, requireConsoleWebPrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

type Context = { params: Promise<{ artifactId: string }> };

export async function GET(request: Request, context: Context): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const { artifactId } = await context.params;
    const value = await getArtifactService().get(parseRequest(artifactIdSchema, artifactId), principal.userId);
    return jsonResponse(request, artifactSummarySchema.parse(value));
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function DELETE(request: Request, context: Context): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const principal = await requireConsoleWebPrincipal(request);
    const { artifactId } = await context.params;
    const parsedArtifactId = parseRequest(artifactIdSchema, artifactId);
    await enforceConsoleRateLimit({
      action: "artifact-delete-owner",
      includeAddress: false,
      key: principal.userId,
      limit: 20,
      request,
      windowSeconds: 60 * 60,
    });
    await enforceConsoleRateLimit({
      action: "artifact-delete",
      key: `${principal.userId}\n${parsedArtifactId}`,
      limit: 10,
      request,
      windowSeconds: 60 * 60,
    });
    const value = await getArtifactService().delete(parsedArtifactId, principal.userId);
    return jsonResponse(request, value);
  } catch (error) {
    return problemResponse(error, request);
  }
}
