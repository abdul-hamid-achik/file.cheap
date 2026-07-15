import { access } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

import { describe, expect, test } from "bun:test";

import openApiDocument from "../../../../openapi.json";

const expectedRoutes = {
  "/api/v1/health": "src/app/api/v1/health/route.ts",
  "/api/v1/openapi.json": "src/app/api/v1/openapi.json/route.ts",
  "/api/v1/stashes": "src/app/api/v1/stashes/route.ts",
  "/api/v1/sync/commits": "src/app/api/v1/sync/commits/route.ts",
  "/api/v1/sync/downloads": "src/app/api/v1/sync/downloads/route.ts",
  "/api/v1/sync/plans": "src/app/api/v1/sync/plans/route.ts",
} as const;

const httpMethods = new Set([
  "delete",
  "get",
  "head",
  "options",
  "patch",
  "post",
  "put",
  "trace",
]);

describe("OpenAPI contract", () => {
  test("publishes OpenAPI 3.1.1 with exactly the control-plane paths", () => {
    expect(openApiDocument.openapi).toBe("3.1.1");
    expect(Object.keys(openApiDocument.paths).sort()).toEqual(
      Object.keys(expectedRoutes).sort(),
    );
  });

  test("assigns one unique operationId to every documented operation", () => {
    const operationIds: string[] = [];

    for (const pathItem of Object.values(openApiDocument.paths)) {
      for (const [method, possibleOperation] of Object.entries(pathItem)) {
        if (!httpMethods.has(method)) {
          continue;
        }
        const operation = possibleOperation as { operationId?: string };
        expect(operation.operationId).toBeString();
        expect(operation.operationId?.length).toBeGreaterThan(0);
        operationIds.push(operation.operationId!);
      }
    }

    expect(new Set(operationIds).size).toBe(operationIds.length);
  });

  test("has a Route Handler file for every documented path", async () => {
    const platformRoot = fileURLToPath(new URL("../../../../", import.meta.url));

    for (const routeFile of Object.values(expectedRoutes)) {
      await access(join(platformRoot, routeFile));
    }
  });

  test("documents strict media type, auth challenge, and retryable contention", () => {
    expect(
      openApiDocument.components.schemas.CreatePlanInput.properties.contentType
        .const,
    ).toBe("application/vnd.filecheap.stash");
    expect(
      openApiDocument.components.responses.Unauthorized.headers[
        "WWW-Authenticate"
      ].$ref,
    ).toBe("#/components/headers/WwwAuthenticate");
    expect(
      openApiDocument.paths["/api/v1/sync/commits"].post.responses["503"]
        .$ref,
    ).toBe("#/components/responses/ServiceUnavailable");
    expect(
      openApiDocument.paths["/api/v1/sync/commits"].post.responses["403"]
        .$ref,
    ).toBe("#/components/responses/InvalidCapability");
    expect(
      openApiDocument.paths["/api/v1/sync/commits"].post.responses["410"]
        .$ref,
    ).toBe("#/components/responses/ExpiredCapability");
  });

  test("serves the same document through the GET Route Handler", async () => {
    const { GET } = await import("@/app/api/v1/openapi.json/route");
    const request = new Request(
      "http://127.0.0.1:3100/api/v1/openapi.json",
      { headers: { "x-request-id": "openapi-contract-01" } },
    );

    const response = GET(request);
    const document = await response.json();

    expect(response.status).toBe(200);
    expect(response.headers.get("x-request-id")).toBe("openapi-contract-01");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
    expect(document).toEqual(openApiDocument);
  });
});
