import { describe, expect, test } from "bun:test";

import type { ConsoleArtifact } from "./artifact-types";
import { deriveArtifactDashboardMetrics } from "./artifact-dashboard-data";

describe("artifact dashboard metrics", () => {
  test("counts only actual ready transfers and future expirations in the next day", () => {
    const now = new Date("2026-07-26T12:00:00.000Z");
    const artifacts: ConsoleArtifact[] = [
      artifact({ availability: "cloud-ready", expiresAt: "2026-07-27T11:00:00.000Z", integrity: "server-sha256", sizeBytes: 100 }),
      artifact({ availability: "unavailable", expiresAt: "2026-07-27T13:00:00.000Z", integrity: "declared", sizeBytes: 50 }),
      artifact({ availability: "cloud-ready", expiresAt: "2026-07-26T11:00:00.000Z", integrity: "server-sha256", sizeBytes: 25 }),
    ];

    expect(deriveArtifactDashboardMetrics(artifacts, now)).toEqual({
      cloudReady: 2,
      expiringSoon: 1,
      totalBytes: 175,
      totalCount: 3,
      verifiedCount: 2,
    });
  });
});

function artifact(overrides: Partial<ConsoleArtifact>): ConsoleArtifact {
  return {
    availability: "cloud-ready",
    id: "art_test",
    integrity: "server-sha256",
    kind: "trace",
    label: "trace",
    producer: { tool: "glyphrun" },
    state: "committed",
    ...overrides,
  };
}
