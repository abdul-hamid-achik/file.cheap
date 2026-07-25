import openApiDocument from "../../../../../openapi.json";

import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export function GET(request: Request): Response {
  try {
    return jsonResponse(request, openApiDocument);
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
