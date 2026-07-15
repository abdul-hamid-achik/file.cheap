import { describe, expect, test } from "bun:test";

import {
  attachResponseMetadata,
  jsonResponse,
  requestIdFor,
} from "@/shared/http/response";

describe("HTTP response metadata", () => {
  test("reflects a valid caller-supplied request ID", async () => {
    const request = requestWithId("trace.client_01:attempt-2");

    const response = jsonResponse(request, { ok: true });

    expect(requestIdFor(request)).toBe("trace.client_01:attempt-2");
    expect(response.headers.get("x-request-id")).toBe(
      "trace.client_01:attempt-2",
    );
    expect(await response.json()).toEqual({ ok: true });
  });

  test("replaces an invalid caller-supplied request ID with one req_ ID", () => {
    const request = requestWithId("invalid request id with spaces");

    const first = requestIdFor(request);
    const second = requestIdFor(request);

    expect(first).toStartWith("req_");
    expect(first).toBe(second);
    expect(first).not.toContain("invalid request id");
  });

  test("adds no-store and nosniff headers to JSON responses", () => {
    const response = jsonResponse(requestWithId("req_from-client"), {
      ok: true,
    });

    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
    expect(response.headers.get("content-type")).toContain("application/json");
  });

  test("attaches metadata without changing the original response semantics", async () => {
    const request = requestWithId("download_01");
    const response = attachResponseMetadata(
      request,
      new Response("archive bytes", {
        headers: { "content-type": "application/octet-stream" },
        status: 206,
        statusText: "Partial Content",
      }),
    );

    expect(response.status).toBe(206);
    expect(response.statusText).toBe("Partial Content");
    expect(response.headers.get("content-type")).toBe(
      "application/octet-stream",
    );
    expect(response.headers.get("x-request-id")).toBe("download_01");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
    expect(await response.text()).toBe("archive bytes");
  });
});

function requestWithId(requestId: string): Request {
  return new Request("http://127.0.0.1:3100/api/v1/test", {
    headers: { "x-request-id": requestId },
  });
}
