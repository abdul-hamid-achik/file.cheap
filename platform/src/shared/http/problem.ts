import type { ZodError, ZodType } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";
import { jsonResponse, requestIdFor } from "@/shared/http/response";

export const controlPlaneJsonLimitBytes = 16 * 1024;

type ProblemDetails = {
  code: string;
  detail: string;
  instance?: string;
  requestId: string;
  status: number;
  title: string;
  type: string;
};

class RequestValidationError extends Error {
  constructor(readonly validationError: ZodError) {
    super("The request did not match its schema");
    this.name = "RequestValidationError";
  }
}

export function problemResponse(error: unknown, request: Request): Response {
  const problem = toProblem(
    error,
    new URL(request.url).pathname,
    requestIdFor(request),
  );

  const headers = new Headers({
    "content-type": "application/problem+json",
  });
  if (problem.code === "unauthorized") {
    headers.set("www-authenticate", 'Bearer realm="filecheap-platform"');
  }
  if (problem.status === 503) {
    headers.set(
      "retry-after",
      String(
        error instanceof PlatformError && error.retryAfterSeconds
          ? Math.max(1, Math.ceil(error.retryAfterSeconds))
          : 1,
      ),
    );
  }

  return jsonResponse(request, problem, {
    headers,
    status: problem.status,
  });
}

export function methodNotAllowedResponse(
  request: Request,
  allowedMethods: readonly string[],
): Response {
  const response = problemResponse(
    new PlatformError({
      code: "method_not_allowed",
      detail: `${request.method} is not supported for this resource.`,
      status: 405,
      title: "Method not allowed",
    }),
    request,
  );
  response.headers.set("allow", allowedMethods.join(", "));
  return response;
}

export function apiNotFoundResponse(request: Request): Response {
  return problemResponse(
    new PlatformError({
      code: "api_route_not_found",
      detail: "The requested platform API route does not exist.",
      status: 404,
      title: "API route not found",
    }),
    request,
  );
}

function toProblem(
  error: unknown,
  instance: string,
  requestId: string,
): ProblemDetails {
  if (error instanceof PlatformError) {
    return {
      code: error.code,
      detail: error.message,
      instance,
      requestId,
      status: error.status,
      title: error.title,
      type: error.type,
    };
  }

  if (error instanceof RequestValidationError) {
    const detail = error.validationError.issues
      .map((issue) => `${issue.path.join(".") || "request"}: ${issue.message}`)
      .join("; ");

    return {
      code: "invalid_request",
      detail,
      instance,
      requestId,
      status: 422,
      title: "Invalid request",
      type: "https://file.cheap/problems/invalid-request",
    };
  }

  console.error(error);
  return {
    code: "internal_error",
    detail: "The platform could not complete the request.",
    instance,
    requestId,
    status: 500,
    title: "Internal server error",
    type: "https://file.cheap/problems/internal-error",
  };
}

export function parseRequest<T>(schema: ZodType<T>, input: unknown): T {
  const result = schema.safeParse(input);
  if (!result.success) {
    throw new RequestValidationError(result.error);
  }
  return result.data;
}

export async function parseJson(request: Request): Promise<unknown> {
  const mediaType = request.headers
    .get("content-type")
    ?.split(";", 1)[0]
    ?.trim()
    .toLowerCase();
  if (mediaType !== "application/json") {
    throw new PlatformError({
      code: "unsupported_media_type",
      detail: "The request Content-Type must be application/json.",
      status: 415,
      title: "Unsupported media type",
    });
  }

  const advertisedLength = request.headers.get("content-length");
  if (advertisedLength !== null) {
    const contentLength = Number(advertisedLength);
    if (
      Number.isFinite(contentLength) &&
      contentLength > controlPlaneJsonLimitBytes
    ) {
      throw payloadTooLargeError();
    }
  }

  try {
    return JSON.parse(await readLimitedText(request));
  } catch (error) {
    if (error instanceof PlatformError) {
      throw error;
    }
    throw new PlatformError({
      code: "invalid_json",
      detail: "The request body must be valid JSON.",
      status: 400,
      title: "Invalid JSON",
    });
  }
}

async function readLimitedText(request: Request): Promise<string> {
  if (!request.body) {
    return "";
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let sizeBytes = 0;

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }

      sizeBytes += value.byteLength;
      if (sizeBytes > controlPlaneJsonLimitBytes) {
        await reader
          .cancel("Control-plane JSON body exceeded its limit")
          .catch(() => undefined);
        throw payloadTooLargeError();
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(sizeBytes);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(body);
}

function payloadTooLargeError(): PlatformError {
  return new PlatformError({
    code: "payload_too_large",
    detail: `Control-plane JSON bodies are limited to ${controlPlaneJsonLimitBytes} bytes.`,
    status: 413,
    title: "Payload too large",
  });
}
