import { describe, expect, test } from "bun:test";
import { z } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";
import {
  parseJson,
  parseRequest,
  problemResponse,
} from "@/shared/http/problem";

describe("RFC 9457 problem responses", () => {
  test("maps semantic Zod failures to 422 with one correlated request ID", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      { headers: { "x-request-id": "semantic-validation-01" } },
    );
    const schema = z.object({ sizeBytes: z.number().int().positive() });
    const validationError = captureError(() =>
      parseRequest(schema, { sizeBytes: 0 }),
    );

    const response = problemResponse(validationError, request);
    const problem = await response.json();

    expect(response.status).toBe(422);
    expect(response.headers.get("content-type")).toBe(
      "application/problem+json",
    );
    expect(response.headers.get("x-request-id")).toBe(
      "semantic-validation-01",
    );
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
    expect(problem).toMatchObject({
      code: "invalid_request",
      instance: "/api/v1/sync/plans",
      requestId: "semantic-validation-01",
      status: 422,
      title: "Invalid request",
    });
    expect(problem.detail).toContain("sizeBytes");
  });

  test("sanitizes internal schema failures instead of treating them as requests", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/stashes",
      { headers: { "x-request-id": "internal-schema-01" } },
    );
    const internalSchema = z.object({ secretCatalogField: z.string() });
    const internalError = captureError(() => internalSchema.parse({}));
    const originalConsoleError = console.error;
    console.error = () => undefined;

    try {
      const response = problemResponse(internalError, request);
      const problem = await response.json();

      expect(response.status).toBe(500);
      expect(problem).toMatchObject({
        code: "internal_error",
        detail: "The platform could not complete the request.",
        requestId: "internal-schema-01",
      });
      expect(JSON.stringify(problem)).not.toContain("secretCatalogField");
    } finally {
      console.error = originalConsoleError;
    }
  });

  test("maps malformed JSON to a typed 400 problem", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/commits",
      {
        body: '{"receipt":',
        headers: {
          "content-type": "application/json",
          "x-request-id": "malformed-json-01",
        },
        method: "POST",
      },
    );
    const parseError = await captureAsyncError(() => parseJson(request));

    expect(parseError).toBeInstanceOf(PlatformError);
    expect(parseError).toMatchObject({ code: "invalid_json", status: 400 });

    const response = problemResponse(parseError, request);
    const problem = await response.json();

    expect(response.status).toBe(400);
    expect(response.headers.get("x-request-id")).toBe("malformed-json-01");
    expect(problem).toMatchObject({
      code: "invalid_json",
      requestId: "malformed-json-01",
      status: 400,
      title: "Invalid JSON",
    });
  });

  test("advertises bearer authentication on 401 responses", () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/stashes",
      { headers: { "x-request-id": "unauthorized-01" } },
    );
    const response = problemResponse(
      new PlatformError({
        code: "unauthorized",
        detail: "A valid bearer token is required.",
        status: 401,
        title: "Unauthorized",
      }),
      request,
    );

    expect(response.status).toBe(401);
    expect(response.headers.get("www-authenticate")).toBe(
      'Bearer realm="filecheap-platform"',
    );
  });

  test("does not advertise bearer authentication for signed grant failures", () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/local-objects",
    );
    const response = problemResponse(
      new PlatformError({
        code: "invalid_grant",
        detail: "The transfer grant is invalid.",
        status: 403,
        title: "Invalid grant",
      }),
      request,
    );

    expect(response.status).toBe(403);
    expect(response.headers.has("www-authenticate")).toBe(false);
  });

  test("marks 503 responses as retryable", () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/commits",
      { headers: { "x-request-id": "catalog-busy-01" } },
    );
    const response = problemResponse(
      new PlatformError({
        code: "catalog_busy",
        detail: "The catalog remained busy. Retry the operation.",
        status: 503,
        title: "Catalog busy",
      }),
      request,
    );

    expect(response.status).toBe(503);
    expect(response.headers.get("retry-after")).toBe("1");
  });
});

function captureError(operation: () => unknown): unknown {
  try {
    operation();
  } catch (error) {
    return error;
  }
  throw new Error("Expected operation to throw");
}

async function captureAsyncError(
  operation: () => Promise<unknown>,
): Promise<unknown> {
  try {
    await operation();
  } catch (error) {
    return error;
  }
  throw new Error("Expected operation to reject");
}
