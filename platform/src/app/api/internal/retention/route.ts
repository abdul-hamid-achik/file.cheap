import { retentionRunReport, type RetentionRun } from "@/features/retention/contracts";
import { getRetentionRunService } from "@/features/retention/factory";
import { requireServiceToken } from "@/shared/auth/bearer";
import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export type RetentionRouteDependencies = Readonly<{
  authorize(request: Request): Promise<void>;
  run(): Promise<RetentionRun>;
}>;

export async function GET(request: Request): Promise<Response> {
  return handleRetentionRequest(request);
}

export async function handleRetentionRequest(
  request: Request,
  dependencies: RetentionRouteDependencies = defaultDependencies(),
): Promise<Response> {
  try {
    await dependencies.authorize(request);
    return jsonResponse(request, retentionRunReport(await dependencies.run()));
  } catch (error) {
    return problemResponse(error, request);
  }
}

function defaultDependencies(): RetentionRouteDependencies {
  return {
    authorize: async (request) => {
      await requireServiceToken(request, "cron");
    },
    run: () => getRetentionRunService().run(),
  };
}

function unsupported(request: Request): Response {
  return methodNotAllowedResponse(request, ["GET"]);
}

export {
  unsupported as DELETE,
  unsupported as HEAD,
  unsupported as OPTIONS,
  unsupported as PATCH,
  unsupported as POST,
  unsupported as PUT,
};
