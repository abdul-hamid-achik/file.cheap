import { commitPlanSchema } from "@/features/sync/contracts";
import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import {
  parseJson,
  parseRequest,
  problemResponse,
} from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request: Request): Promise<Response> {
  try {
    requireApiToken(request);
    const input = parseRequest(commitPlanSchema, await parseJson(request));
    return jsonResponse(request, await getSyncService().commitPlan(input));
  } catch (error) {
    return problemResponse(error, request);
  }
}
