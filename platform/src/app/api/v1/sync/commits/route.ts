import { commitPlanSchema } from "@/features/sync/contracts";
import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import { parseJson, problemResponse } from "@/shared/http/problem";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request: Request): Promise<Response> {
  try {
    requireApiToken(request);
    const input = commitPlanSchema.parse(await parseJson(request));
    return Response.json(await getSyncService().commitPlan(input));
  } catch (error) {
    return problemResponse(error, request);
  }
}
