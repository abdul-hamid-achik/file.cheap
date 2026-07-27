import { describe, expect, test } from "bun:test";

import { handleRetentionRequest } from "@/app/api/internal/retention/route";
import { handleRetentionHealthRequest } from "@/app/api/internal/retention/health/route";
import {
  emptyRetentionCounters,
  type RetentionHealth,
  type RetentionRun,
} from "@/features/retention/contracts";
import { PlatformError } from "@/shared/errors/platform-error";

const now = new Date("2026-07-26T18:00:00.000Z");
const runId = "rtn_00000000-0000-4000-8000-000000000001";

describe("private retention routes", () => {
  test("authenticates before running and returns only the bounded run report", async () => {
    const order: string[] = [];
    const response = await handleRetentionRequest(request(), {
      authorize: async () => { order.push("authorize"); },
      run: async () => {
        order.push("run");
        return successfulRun();
      },
    });

    expect(response.status).toBe(200);
    expect(order).toEqual(["authorize", "run"]);
    expect(await response.json()).toEqual({
      counters: emptyRetentionCounters(),
      failedAreas: [],
      finishedAt: now.toISOString(),
      oldestDueAt: null,
      runId,
      startedAt: now.toISOString(),
      status: "succeeded",
      version: "filecheap-retention-run/1",
    });
  });

  test("returns retryable 503 only after the service persisted a partial run", async () => {
    let persisted = false;
    const response = await handleRetentionRequest(request(), {
      authorize: async () => undefined,
      run: async () => {
        persisted = true;
        throw new PlatformError({
          code: "retention_incomplete",
          detail: "The private retention run finished with status 'partial'.",
          retryAfterSeconds: 60,
          status: 503,
          title: "Retention incomplete",
        });
      },
    });

    expect(persisted).toBe(true);
    expect(response.status).toBe(503);
    expect(response.headers.get("retry-after")).toBe("60");
    expect(await response.json()).toMatchObject({ code: "retention_incomplete" });
  });

  test("keeps health under the authenticated internal route", async () => {
    const unauthorized = await handleRetentionHealthRequest(request(), {
      authorize: async () => {
        throw new PlatformError({
          code: "unauthorized",
          detail: "A valid private service credential is required.",
          status: 401,
          title: "Unauthorized",
        });
      },
      health: async () => {
        throw new Error("health must not run");
      },
    });
    expect(unauthorized.status).toBe(401);

    const health: RetentionHealth = {
      activeRunCount: 0,
      counters: emptyRetentionCounters(),
      lastFinishedAt: now,
      lastRunId: runId,
      lastStartedAt: now,
      oldestDueAt: null,
      status: "succeeded",
    };
    const response = await handleRetentionHealthRequest(request(), {
      authorize: async () => undefined,
      health: async () => health,
    });
    expect(response.status).toBe(200);
    expect(await response.json()).toMatchObject({
      lastRunId: runId,
      status: "succeeded",
      version: "filecheap-retention-health/1",
    });
  });
});

function request(): Request {
  return new Request("https://file.cheap/api/internal/retention");
}

function successfulRun(): RetentionRun {
  return {
    counters: emptyRetentionCounters(),
    failedAreas: [],
    finishedAt: now,
    heartbeatAt: now,
    id: runId,
    oldestDueAt: null,
    startedAt: now,
    status: "succeeded",
  };
}
