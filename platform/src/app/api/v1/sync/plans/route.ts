import { createPlanSchema } from "@/features/sync/contracts";
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
    const input = parseRequest(createPlanSchema, await parseJson(request));
    return jsonResponse(request, await getSyncService().createPlan(input), {
      status: 201,
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}
