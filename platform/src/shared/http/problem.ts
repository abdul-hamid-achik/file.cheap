import { ZodError } from "zod";

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

export function problemResponse(error: unknown, request: Request): Response {
  const problem = toProblem(
    error,
    new URL(request.url).pathname,
    requestIdFor(request),
  );

  return jsonResponse(request, problem, {
    headers: { "content-type": "application/problem+json" },
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

  if (error instanceof ZodError) {
    const detail = error.issues
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
