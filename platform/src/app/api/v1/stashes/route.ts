import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import { problemResponse } from "@/shared/http/problem";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<Response> {
  try {
    requireApiToken(request);
    return Response.json({
      stashes: await getSyncService().listStashes(),
      version: "filecheap-sync/1",
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}
