import { apiNotFoundResponse } from "@/shared/http/problem";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

function notFound(request: Request): Response {
  return apiNotFoundResponse(request);
}

export {
  notFound as DELETE,
  notFound as GET,
  notFound as HEAD,
  notFound as OPTIONS,
  notFound as PATCH,
  notFound as POST,
  notFound as PUT,
};
