import { describe, expect, test } from "bun:test";

import {
  inboundReplayRetentionMilliseconds,
  InMemoryInboundReplayRepository,
  maxInboundReplayAttempts,
} from "@/features/mail/replay-repository";

describe("inbound replay repository", () => {
  test("requires the current random lease token to finalize a replay", async () => {
    const repository = new InMemoryInboundReplayRepository();
    const now = new Date("2026-07-25T00:00:00.000Z");
    const claim = await repository.claim({
      emailIdSha256: "a".repeat(64),
      now,
      svixIdSha256: "b".repeat(64),
    });
    if (claim.state !== "claimed") throw new Error("Expected a replay lease");

    await expect(repository.markForwarded(
      "a".repeat(64),
      "wrong-lease",
      now,
    )).resolves.toBe(false);
    await expect(repository.claim({
      emailIdSha256: "a".repeat(64),
      now,
      svixIdSha256: "c".repeat(64),
    })).resolves.toEqual({ state: "in_progress" });
    await expect(repository.markForwarded(
      "a".repeat(64),
      claim.leaseToken,
      now,
    )).resolves.toBe(true);
    await expect(repository.claim({
      emailIdSha256: "a".repeat(64),
      now,
      svixIdSha256: "c".repeat(64),
    })).resolves.toEqual({ state: "duplicate" });
  });

  test("removes only expired digest rows so a new 90-day retention window can begin", async () => {
    const repository = new InMemoryInboundReplayRepository();
    const now = new Date("2026-07-25T00:00:00.000Z");
    await repository.claim({
      emailIdSha256: "a".repeat(64),
      now,
      svixIdSha256: "b".repeat(64),
    });
    const expired = new Date(now.getTime() + inboundReplayRetentionMilliseconds + 1);
    await repository.cleanup(expired);
    await expect(repository.claim({
      emailIdSha256: "a".repeat(64),
      now: expired,
      svixIdSha256: "c".repeat(64),
    })).resolves.toMatchObject({ state: "claimed" });
  });

  test("terminally rejects a replay after bounded failed processing leases", async () => {
    expect(maxInboundReplayAttempts).toBe(8);
    const repository = new InMemoryInboundReplayRepository();
    let now = new Date("2026-07-25T00:00:00.000Z");
    for (let attempt = 1; attempt <= maxInboundReplayAttempts; attempt += 1) {
      const claim = await repository.claim({
        emailIdSha256: "a".repeat(64),
        now,
        svixIdSha256: "b".repeat(64),
      });
      if (claim.state !== "claimed") throw new Error("Expected a retry lease");
      await repository.release("a".repeat(64), claim.leaseToken, now);
      now = new Date(now.getTime() + 1);
    }
    await expect(repository.claim({
      emailIdSha256: "a".repeat(64),
      now,
      svixIdSha256: "b".repeat(64),
    })).resolves.toEqual({ state: "duplicate" });
  });
});
