import { randomUUID } from "node:crypto";

const acceptedRequestId = /^[A-Za-z0-9._:-]{1,128}$/;
const generatedRequestIds = new WeakMap<Request, string>();

export function requestIdFor(request: Request): string {
  const existing = generatedRequestIds.get(request);
  if (existing) {
    return existing;
  }

  const supplied = request.headers.get("x-request-id");
  const requestId = supplied && acceptedRequestId.test(supplied)
    ? supplied
    : `req_${randomUUID()}`;
  generatedRequestIds.set(request, requestId);
  return requestId;
}

export function jsonResponse(
  request: Request,
  body: unknown,
  init: ResponseInit = {},
): Response {
  const headers = responseHeaders(request, init.headers);
  return Response.json(body, { ...init, headers });
}

export function attachResponseMetadata(
  request: Request,
  response: Response,
): Response {
  const headers = responseHeaders(request, response.headers);
  return new Response(response.body, {
    headers,
    status: response.status,
    statusText: response.statusText,
  });
}

function responseHeaders(request: Request, initial?: HeadersInit): Headers {
  const headers = new Headers(initial);
  headers.set("cache-control", "no-store");
  headers.set("x-content-type-options", "nosniff");
  headers.set("x-request-id", requestIdFor(request));
  return headers;
}
