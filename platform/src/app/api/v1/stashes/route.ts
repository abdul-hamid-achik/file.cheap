import { stashListSchema } from "@/features/sync/contracts";
import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<Response> {
  try {
    requireApiToken(request);
    return jsonResponse(request, stashListSchema.parse({
      stashes: await getSyncService().listStashes(request.signal),
      version: "filecheap-sync/1",
    }));
  } catch (error) {
    return problemResponse(error, request);
  }
}

function unsupportedMethod(request: Request): Response {
  return methodNotAllowedResponse(request, ["GET", "HEAD"]);
}

export {
  unsupportedMethod as DELETE,
  unsupportedMethod as OPTIONS,
  unsupportedMethod as PATCH,
  unsupportedMethod as POST,
  unsupportedMethod as PUT,
};
