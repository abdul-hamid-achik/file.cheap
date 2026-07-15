import type { ZodError, ZodType } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";
import { jsonResponse, requestIdFor } from "@/shared/http/response";

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
    headers.set("retry-after", "1");
  }

  return jsonResponse(request, problem, {
    headers,
    status: problem.status,
  });
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
  try {
    return await request.json();
  } catch {
    throw new PlatformError({
      code: "invalid_json",
      detail: "The request body must be valid JSON.",
      status: 400,
      title: "Invalid JSON",
    });
  }
}
