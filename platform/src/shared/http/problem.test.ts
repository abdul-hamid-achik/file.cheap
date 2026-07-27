import { describe, expect, test } from "bun:test";
import { z } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";
import {
  apiNotFoundResponse,
  controlPlaneJsonLimitBytes,
  methodNotAllowedResponse,
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
    const logged: unknown[] = [];
    console.error = (...values: unknown[]) => { logged.push(...values); };

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
      // The point of this assertion: a failing schema names the field it
      // rejected, and that name is catalog data. It must not reach the log.
      expect(JSON.stringify(logged)).not.toContain("secretCatalogField");
      // requestId ties the line to the response the caller already holds, so
      // an operator can find the one failure they were told about. It is not
      // new information — it is the same id the body carries.
      expect(logged).toEqual([
        {
          event: "platform_request_failed",
          errorName: "ZodError",
          requestId: "internal-schema-01",
        },
      ]);
    } finally {
      console.error = originalConsoleError;
    }
  });

  test("carries a structured reason without carrying the signed URL it came from", async () => {
    // A real 500 on the artifact plan route logged `errorName: "Error"` and
    // nothing else, and three throw sites shared that name and that sentence,
    // so production could not be told which one fired. The reason travels; the
    // URL, the delegation token and the signature never do.
    class GrantShapeStub extends Error {
      readonly reason = "query-unexpected";
      readonly unexpectedQueryKeys = ["vercel-blob-new-parameter"];
      constructor() {
        super("https://vercel.com/api/blob/?vercel-blob-signature=SUPERSECRETSIG");
        this.name = "BlobGrantShapeError";
      }
    }
    const request = new Request("https://file.cheap/api/v1/artifacts/plans", {
      headers: { "x-request-id": "grant-shape-01" },
      method: "POST",
    });
    const originalConsoleError = console.error;
    const logged: unknown[] = [];
    console.error = (...values: unknown[]) => { logged.push(...values); };

    try {
      const response = problemResponse(new GrantShapeStub(), request);
      expect(response.status).toBe(500);
      expect(logged).toEqual([
        {
          event: "platform_request_failed",
          errorName: "BlobGrantShapeError",
          requestId: "grant-shape-01",
          reason: "query-unexpected",
          unexpectedQueryKeys: ["vercel-blob-new-parameter"],
        },
      ]);
      // The message is a URL bearing a signature. Nothing reaches into it.
      expect(JSON.stringify(logged)).not.toContain("SUPERSECRETSIG");
      expect(JSON.stringify(logged)).not.toContain("vercel.com/api/blob");
      expect(JSON.stringify(await response.json())).not.toContain("SUPERSECRETSIG");
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

  test("requires application/json while accepting media type parameters", async () => {
    const unsupportedRequest = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      {
        body: "{}",
        headers: { "content-type": "text/plain" },
        method: "POST",
      },
    );
    const unsupportedError = await captureAsyncError(() =>
      parseJson(unsupportedRequest),
    );
    expect(unsupportedError).toMatchObject({
      code: "unsupported_media_type",
      status: 415,
    });

    const jsonRequest = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      {
        body: '{"ok":true}',
        headers: { "content-type": "Application/JSON; Charset=UTF-8" },
        method: "POST",
      },
    );
    await expect(parseJson(jsonRequest)).resolves.toEqual({ ok: true });
  });

  test("rejects an advertised JSON body over the control-plane limit", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      {
        body: "{}",
        headers: {
          "content-length": String(controlPlaneJsonLimitBytes + 1),
          "content-type": "application/json",
        },
        method: "POST",
      },
    );

    const error = await captureAsyncError(() => parseJson(request));
    expect(error).toMatchObject({ code: "payload_too_large", status: 413 });
  });

  test("enforces the JSON limit while streaming when length is absent", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/commits",
      {
        body: new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(
              new TextEncoder().encode(
                `{"receipt":"${"x".repeat(controlPlaneJsonLimitBytes)}"}`,
              ),
            );
            controller.close();
          },
        }),
        // Node requires this when a Request carries a streaming body.
        duplex: "half",
        headers: { "content-type": "application/json" },
        method: "POST",
      } as RequestInit & { duplex: "half" },
    );

    const error = await captureAsyncError(() => parseJson(request));
    expect(error).toMatchObject({ code: "payload_too_large", status: 413 });
  });

  test("accepts a valid body at the exact byte limit", async () => {
    const prefix = '{"value":"';
    const suffix = '"}';
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      {
        body: `${prefix}${"x".repeat(
          controlPlaneJsonLimitBytes - prefix.length - suffix.length,
        )}${suffix}`,
        headers: { "content-type": "application/json" },
        method: "POST",
      },
    );

    const body = await parseJson(request);
    expect((body as { value: string }).value.length).toBe(
      controlPlaneJsonLimitBytes - prefix.length - suffix.length,
    );
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

  test("preserves a dependency-specific retry delay", () => {
    const response = problemResponse(
      new PlatformError({
        code: "storage_rate_limited",
        detail: "Retry later.",
        retryAfterSeconds: 7,
        status: 503,
        title: "Storage rate limited",
      }),
      new Request("http://127.0.0.1:3100/api/v1/stashes"),
    );

    expect(response.headers.get("retry-after")).toBe("7");
  });

  test("preserves an explicit retry delay on a transient conflict", () => {
    const response = problemResponse(
      new PlatformError({
        code: "idempotency_reconciling",
        detail: "Retry the same immutable plan.",
        retryAfterSeconds: 2,
        status: 409,
        title: "Upload plan reconciling",
      }),
      new Request("https://file.cheap/api/v1/artifacts/plans"),
    );

    expect(response.status).toBe(409);
    expect(response.headers.get("retry-after")).toBe("2");
  });

  test("returns correlated problem details for unsupported API methods", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      {
        headers: { "x-request-id": "wrong-method-01" },
        method: "DELETE",
      },
    );
    const response = methodNotAllowedResponse(request, ["POST"]);

    expect(response.status).toBe(405);
    expect(response.headers.get("allow")).toBe("POST");
    expect(response.headers.get("content-type")).toBe(
      "application/problem+json",
    );
    expect(await response.json()).toMatchObject({
      code: "method_not_allowed",
      requestId: "wrong-method-01",
      status: 405,
    });
  });

  test("returns JSON problem details for unknown API routes", async () => {
    const request = new Request("http://127.0.0.1:3100/api/v1/nope", {
      headers: { "x-request-id": "missing-route-01" },
    });
    const response = apiNotFoundResponse(request);

    expect(response.status).toBe(404);
    expect(response.headers.get("content-type")).toBe(
      "application/problem+json",
    );
    expect(await response.json()).toMatchObject({
      code: "api_route_not_found",
      requestId: "missing-route-01",
      status: 404,
    });
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
