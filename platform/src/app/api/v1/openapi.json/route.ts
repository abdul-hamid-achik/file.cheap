import openApiDocument from "../../../../../openapi.json";

import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export function GET(request: Request): Response {
  return jsonResponse(request, openApiDocument);
}
