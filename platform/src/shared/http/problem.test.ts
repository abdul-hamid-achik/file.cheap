import { describe, expect, test } from "bun:test";
import { z } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";
import { parseJson, problemResponse } from "@/shared/http/problem";

describe("RFC 9457 problem responses", () => {
  test("maps semantic Zod failures to 422 with one correlated request ID", async () => {
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/sync/plans",
      { headers: { "x-request-id": "semantic-validation-01" } },
    );
    const schema = z.object({ sizeBytes: z.number().int().positive() });
    const validationError = captureError(() => schema.parse({ sizeBytes: 0 }));

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
