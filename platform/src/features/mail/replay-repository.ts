import { createHash, randomUUID } from "node:crypto";

export const inboundReplayRetentionMilliseconds = 90 * 24 * 60 * 60 * 1_000;
export const inboundProcessingLeaseMilliseconds = 2 * 60 * 1_000;
// Resend can deliver a webhook up to eight times, so retain a lease for each
// provider attempt before terminally suppressing further retries.
export const maxInboundReplayAttempts = 8;
export const inboundReplayCleanupBatchSize = 100;

export type InboundReplayClaim =
  | { leaseToken: string; state: "claimed" }
  | { state: "duplicate" }
  | { state: "in_progress" };

export interface InboundReplayRepository {
  claim(input: {
    emailIdSha256: string;
    now: Date;
    svixIdSha256: string;
  }): Promise<InboundReplayClaim>;
  markForwarded(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean>;
  markIgnored(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean>;
  markAmbiguous(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean>;
  markRejected(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean>;
  release(emailIdSha256: string, leaseToken: string, now: Date): Promise<void>;
  cleanup(now: Date): Promise<number>;
}

export function replayDigest(value: string): string {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

type InMemoryReplay = {
  attempts: number;
  emailIdSha256: string;
  expiresAt: Date;
  leaseToken: string | null;
  processingLeaseExpiresAt: Date | null;
  status: "ambiguous" | "forwarded" | "ignored" | "processing" | "rejected";
  svixIdSha256: string;
};

export class InMemoryInboundReplayRepository implements InboundReplayRepository {
  private readonly byEmail = new Map<string, InMemoryReplay>();
  private readonly bySvix = new Map<string, InMemoryReplay>();

  async claim(input: {
    emailIdSha256: string;
    now: Date;
    svixIdSha256: string;
  }): Promise<InboundReplayClaim> {
    const existing = this.byEmail.get(input.emailIdSha256) ??
      this.bySvix.get(input.svixIdSha256);
    if (!existing) {
      const leaseToken = newReplayId();
      const record: InMemoryReplay = {
        attempts: 1,
        emailIdSha256: input.emailIdSha256,
        expiresAt: new Date(input.now.getTime() + inboundReplayRetentionMilliseconds),
        leaseToken,
        processingLeaseExpiresAt: new Date(
          input.now.getTime() + inboundProcessingLeaseMilliseconds,
        ),
        status: "processing",
        svixIdSha256: input.svixIdSha256,
      };
      this.byEmail.set(record.emailIdSha256, record);
      this.bySvix.set(record.svixIdSha256, record);
      return { leaseToken, state: "claimed" };
    }
    if (existing.status !== "processing") return { state: "duplicate" };
    if (
      existing.processingLeaseExpiresAt !== null &&
      existing.processingLeaseExpiresAt > input.now
    ) {
      return { state: "in_progress" };
    }
    if (existing.attempts >= maxInboundReplayAttempts) {
      existing.status = "rejected";
      existing.leaseToken = null;
      existing.processingLeaseExpiresAt = null;
      return { state: "duplicate" };
    }
    const leaseToken = newReplayId();
    existing.attempts += 1;
    existing.leaseToken = leaseToken;
    existing.processingLeaseExpiresAt = new Date(
      input.now.getTime() + inboundProcessingLeaseMilliseconds,
    );
    return { leaseToken, state: "claimed" };
  }

  async markForwarded(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "forwarded", now);
  }

  async markIgnored(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "ignored", now);
  }

  async markAmbiguous(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "ambiguous", now);
  }

  async markRejected(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "rejected", now);
  }

  async release(emailIdSha256: string, leaseToken: string, now: Date): Promise<void> {
    const record = this.byEmail.get(emailIdSha256);
    if (record?.status === "processing" && record.leaseToken === leaseToken) {
      record.processingLeaseExpiresAt = now;
    }
  }

  async cleanup(now: Date): Promise<number> {
    let deleted = 0;
    const candidates = [...this.byEmail.values()]
      .filter((record) => record.expiresAt <= now)
      .sort((left, right) => {
        const time = left.expiresAt.getTime() - right.expiresAt.getTime();
        return time === 0
          ? left.emailIdSha256.localeCompare(right.emailIdSha256)
          : time;
      })
      .slice(0, inboundReplayCleanupBatchSize);
    for (const record of candidates) {
      this.byEmail.delete(record.emailIdSha256);
      this.bySvix.delete(record.svixIdSha256);
      deleted += 1;
    }
    return deleted;
  }

  private complete(
    emailIdSha256: string,
    leaseToken: string,
    status: "ambiguous" | "forwarded" | "ignored" | "rejected",
    now: Date,
  ): boolean {
    void now;
    const record = this.byEmail.get(emailIdSha256);
    if (
      !record ||
      record.status !== "processing" ||
      record.leaseToken !== leaseToken
    ) return false;
    record.status = status;
    record.leaseToken = null;
    record.processingLeaseExpiresAt = null;
    return true;
  }
}

export function newReplayId(): string {
  return randomUUID();
}
