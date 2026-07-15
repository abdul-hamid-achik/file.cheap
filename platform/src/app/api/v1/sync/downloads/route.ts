import { createDownloadSchema } from "@/features/sync/contracts";
import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import { parseJson, problemResponse } from "@/shared/http/problem";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request: Request): Promise<Response> {
  try {
    requireApiToken(request);
    const input = createDownloadSchema.parse(await parseJson(request));
    return Response.json(await getSyncService().createDownload(input), { status: 201 });
  } catch (error) {
    return problemResponse(error, request);
  }
}
