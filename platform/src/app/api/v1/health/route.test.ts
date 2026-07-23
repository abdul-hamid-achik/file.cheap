import { afterEach, describe, expect, test } from "bun:test";

import { GET } from "@/app/api/v1/health/route";
import { resetObjectStoreForTests } from "@/platform/storage/factory";
import { resetConfigForTests } from "@/shared/config/env";

const originalVercel = process.env.VERCEL;
const originalStorageDriver = process.env.PLATFORM_STORAGE_DRIVER;
const originalLabEnabled = process.env.PLATFORM_RECOVERY_LAB_ENABLED;
const originalConsoleError = console.error;

afterEach(() => {
  restoreEnvironment("VERCEL", originalVercel);
  restoreEnvironment("PLATFORM_STORAGE_DRIVER", originalStorageDriver);
  restoreEnvironment(
    "PLATFORM_RECOVERY_LAB_ENABLED",
    originalLabEnabled,
  );
  resetObjectStoreForTests();
  resetConfigForTests();
  console.error = originalConsoleError;
});

describe("health Route Handler", () => {
  test("reports the public site healthy without reading storage configuration when the lab is disabled", async () => {
    process.env.VERCEL = "1";
    process.env.PLATFORM_STORAGE_DRIVER = "local";
    delete process.env.PLATFORM_RECOVERY_LAB_ENABLED;
    resetObjectStoreForTests();
    resetConfigForTests();

    const response = GET(
      new Request("http://127.0.0.1:3100/api/v1/health", {
        headers: { "x-request-id": "health-public-site" },
      }),
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("x-request-id")).toBe("health-public-site");
    expect(await response.json()).toMatchObject({
      deployment: "public-site",
      recoveryLab: "disabled",
      status: "ok",
      storage: "disabled",
      storageVerification: "not-applicable",
    });
  });

  test("returns a typed problem when enabled lab configuration is invalid", async () => {
    process.env.VERCEL = "1";
    process.env.PLATFORM_STORAGE_DRIVER = "local";
    process.env.PLATFORM_RECOVERY_LAB_ENABLED = "true";
    console.error = () => undefined;
    resetObjectStoreForTests();
    resetConfigForTests();

    const response = GET(
      new Request("http://127.0.0.1:3100/api/v1/health", {
        headers: { "x-request-id": "health-invalid-config" },
      }),
    );
    const problem = await response.json();

    expect(response.status).toBe(500);
    expect(response.headers.get("content-type")).toContain(
      "application/problem+json",
    );
    expect(response.headers.get("x-request-id")).toBe("health-invalid-config");
    expect(problem).toMatchObject({
      code: "internal_error",
      requestId: "health-invalid-config",
      status: 500,
    });
  });
});

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
