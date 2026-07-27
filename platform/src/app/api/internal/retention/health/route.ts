import { retentionHealthReport, type RetentionHealth } from "@/features/retention/contracts";
import { getRetentionRunService } from "@/features/retention/factory";
import { requireServiceToken } from "@/shared/auth/bearer";
import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export type RetentionHealthRouteDependencies = Readonly<{
  authorize(request: Request): Promise<void>;
  health(): Promise<RetentionHealth>;
}>;

export async function GET(request: Request): Promise<Response> {
  return handleRetentionHealthRequest(request);
}

export async function handleRetentionHealthRequest(
  request: Request,
  dependencies: RetentionHealthRouteDependencies = defaultDependencies(),
): Promise<Response> {
  try {
    await dependencies.authorize(request);
    return jsonResponse(
      request,
      retentionHealthReport(await dependencies.health()),
    );
  } catch (error) {
    return problemResponse(error, request);
  }
}

function defaultDependencies(): RetentionHealthRouteDependencies {
  return {
    authorize: async (request) => {
      await requireServiceToken(request, "cron");
    },
    health: () => getRetentionRunService().health(),
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
