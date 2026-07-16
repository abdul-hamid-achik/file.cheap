import { createDownloadSchema } from "@/features/sync/contracts";
import { getSyncService } from "@/features/sync/factory";
import { requireApiToken } from "@/shared/auth/bearer";
import {
  methodNotAllowedResponse,
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
    const input = parseRequest(createDownloadSchema, await parseJson(request));
    return jsonResponse(request, await getSyncService().createDownload(input), {
      status: 201,
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}

function unsupportedMethod(request: Request): Response {
  return methodNotAllowedResponse(request, ["POST"]);
}

export {
  unsupportedMethod as DELETE,
  unsupportedMethod as GET,
  unsupportedMethod as HEAD,
  unsupportedMethod as OPTIONS,
  unsupportedMethod as PATCH,
  unsupportedMethod as PUT,
};
