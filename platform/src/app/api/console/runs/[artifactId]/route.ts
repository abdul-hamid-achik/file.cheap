import { artifactIdSchema } from "@/features/artifacts/contracts";
import { runSummarySchema } from "@/features/runs/contracts";
import { getRunService } from "@/features/runs/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

type Context = { params: Promise<{ artifactId: string }> };

export async function GET(request: Request, context: Context): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    const { artifactId } = await context.params;
    const run = await getRunService().get(artifactIdSchema.parse(artifactId), principal.userId);
    return jsonResponse(request, runSummarySchema.parse(run));
  } catch (error) {
    return problemResponse(error, request);
  }
}
