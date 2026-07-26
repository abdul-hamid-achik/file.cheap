import { describe, expect, test } from "bun:test";

import {
  type ForwardMessage,
  type InboundMailClient,
  processInboundEmail,
} from "@/features/mail/inbound";
import { InboundContentRejectedError } from "@/features/mail/errors";
import {
  InMemoryInboundReplayRepository,
  replayDigest,
} from "@/features/mail/replay-repository";
import { ResendReceivingClient } from "@/platform/mail/resend-receiving-client";

function receivedEvent(overrides: Record<string, unknown> = {}) {
  return {
    type: "email.received",
    data: {
      email_id: "received-email-1",
      from: "Sender <sender@example.test>",
      to: ["hello@file.cheap"],
      ...overrides,
    },
  };
}

function client(overrides: Partial<InboundMailClient> = {}): InboundMailClient {
  return {
    getMetadata: async (id) => ({
      from: "Sender <sender@example.test>",
      html: "<p>Hello from a fixture.</p>",
      id,
      attachmentsPresent: false,
      receivedFor: ["hello@file.cheap"],
      replyTo: ["Replies <reply@example.test>"],
      subject: "A bounded message",
      text: "Hello from a fixture.",
      to: ["hello@file.cheap"],
    }),
    send: async () => undefined,
    ...overrides,
  };
}

function input(
  mailClient: InboundMailClient,
  replay = new InMemoryInboundReplayRepository(),
) {
  return {
    client: mailClient,
    forwardTo: "owner@example.test",
    replay,
    svixId: "msg_fixture_1",
  };
}

class FinalizationFailureReplay extends InMemoryInboundReplayRepository {
  ambiguousCalls = 0;
  releaseCalls = 0;

  constructor(private readonly outcome: "false" | "throw") {
    super();
  }

  override async markForwarded(
    emailIdSha256: string,
    leaseToken: string,
    now: Date,
  ): Promise<boolean> {
    void emailIdSha256;
    void leaseToken;
    void now;
    if (this.outcome === "throw") throw new Error("ledger unavailable");
    return false;
  }

  override async markAmbiguous(
    emailIdSha256: string,
    leaseToken: string,
    now: Date,
  ): Promise<boolean> {
    this.ambiguousCalls += 1;
    return super.markAmbiguous(emailIdSha256, leaseToken, now);
  }

  override async release(
    emailIdSha256: string,
    leaseToken: string,
    now: Date,
  ): Promise<void> {
    this.releaseCalls += 1;
    return super.release(emailIdSha256, leaseToken, now);
  }
}

class AmbiguityTrackingReplay extends InMemoryInboundReplayRepository {
  ambiguousCalls = 0;
  releaseCalls = 0;

  override async markAmbiguous(
    emailIdSha256: string,
    leaseToken: string,
    now: Date,
  ): Promise<boolean> {
    this.ambiguousCalls += 1;
    return super.markAmbiguous(emailIdSha256, leaseToken, now);
  }

  override async release(
    emailIdSha256: string,
    leaseToken: string,
    now: Date,
  ): Promise<void> {
    this.releaseCalls += 1;
    return super.release(emailIdSha256, leaseToken, now);
  }
}

function resendMetadata(id: string) {
  return {
    attachments: [],
    bcc: null,
    cc: null,
    created_at: "2026-07-25T00:00:00.000Z",
    from: "sender@example.test",
    headers: null,
    html: null,
    id,
    message_id: "<fixture@example.test>",
    object: "email",
    received_for: ["hello@file.cheap"],
    reply_to: [],
    subject: "Fixture",
    text: "Fixture body",
    to: ["hello@file.cheap"],
  };
}

describe("inbound email policy", () => {
  test("fetches bounded content after filtering and sends with a fixed sender and reply-to", async () => {
    const messages: ForwardMessage[] = [];
    await expect(processInboundEmail(receivedEvent(), input(client({
      send: async (message) => {
        messages.push(message);
      },
    })))).resolves.toEqual({ action: "forwarded" });

    expect(messages).toEqual([expect.objectContaining({
      from: "file.cheap Inbox <inbox@file.cheap>",
      idempotencyKey: `inbound-forward/filecheap/${replayDigest("received-email-1")}`,
      replyTo: "reply@example.test",
      subject: "[External] A bounded message",
      text: "[External email]\n\nHello from a fixture.",
      to: "owner@example.test",
    })]);
  });

  test("omits attachments and adds a neutral forwarding notice", async () => {
    const messages: ForwardMessage[] = [];
    await processInboundEmail(receivedEvent(), input(client({
      getMetadata: async (id) => ({
        attachmentsPresent: true,
        from: "sender@example.test",
        id,
        receivedFor: ["hello@file.cheap"],
        replyTo: [],
        subject: "Files are omitted",
        text: "A file was attached.",
        to: ["hello@file.cheap"],
      }),
      send: async (message) => {
        messages.push(message);
      },
    })));
    expect(messages[0]?.text).toContain("Attachments were omitted");
    expect(messages[0]).not.toHaveProperty("attachments");
  });

  test("acknowledges unrelated and looping global events before state or provider access", async () => {
    let calls = 0;
    const failIfCalled = client({
      getMetadata: async () => {
        calls += 1;
        throw new Error("must not fetch");
      },
    });
    const replay = new InMemoryInboundReplayRepository();
    await expect(processInboundEmail(
      receivedEvent({ to: ["other@file.cheap"] }),
      input(failIfCalled, replay),
    )).resolves.toEqual({ action: "ignored", reason: "recipient" });
    await expect(processInboundEmail(
      receivedEvent({ from: "inbox@file.cheap" }),
      input(failIfCalled, replay),
    )).resolves.toEqual({ action: "ignored", reason: "loop" });
    expect(calls).toBe(0);
  });

  test("ignores unrelated team-global events before reading forwarding configuration", async () => {
    await expect(processInboundEmail(
      receivedEvent({ to: ["other@file.cheap"] }),
      {
        client: client(),
        forwardTo: "not-an-email",
        replay: new InMemoryInboundReplayRepository(),
        svixId: "msg_unrelated",
      },
    )).resolves.toEqual({ action: "ignored", reason: "recipient" });
  });

  test("rejects conflicting optional envelope recipients before provider access", async () => {
    let calls = 0;
    await expect(processInboundEmail(
      receivedEvent({ received_for: ["other@file.cheap"] }),
      input(client({
        getMetadata: async () => {
          calls += 1;
          throw new Error("must not fetch");
        },
      })),
    )).resolves.toEqual({ action: "ignored", reason: "recipient" });
    expect(calls).toBe(0);
  });

  test("rejects mixed signed recipients and missing canonical recipients", async () => {
    let calls = 0;
    const guardedClient = client({
      getMetadata: async () => {
        calls += 1;
        throw new Error("must not fetch");
      },
    });
    await expect(processInboundEmail(
      receivedEvent({ to: ["hello@file.cheap", "other@file.cheap"] }),
      input(guardedClient),
    )).resolves.toEqual({ action: "ignored", reason: "recipient" });
    await expect(processInboundEmail(
      receivedEvent({ to: undefined }),
      input(guardedClient),
    )).rejects.toMatchObject({ code: "invalid_inbound_email_event" });
    expect(calls).toBe(0);
  });

  test("rejects conflicting authenticated recipient metadata", async () => {
    await expect(processInboundEmail(
      receivedEvent(),
      input(client({
        getMetadata: async (id) => ({
          attachmentsPresent: false,
          from: "sender@example.test",
          id,
          receivedFor: ["other@file.cheap"],
          replyTo: [],
          subject: "Fixture",
          text: "Fixture body",
          to: ["hello@file.cheap"],
        }),
      })),
    )).resolves.toEqual({ action: "ignored", reason: "recipient" });
  });

  test("rejects malformed inbound events before processing", async () => {
    await expect(processInboundEmail(
      receivedEvent({ email_id: "../unsafe" }),
      input(client()),
    )).rejects.toMatchObject({ code: "invalid_inbound_email_event" });
  });

  test("uses a processing lease and durable digest replay suppression for concurrent deliveries", async () => {
    let releaseSend: (() => void) | undefined;
    const sending = new Promise<void>((resolve) => {
      releaseSend = resolve;
    });
    let sent = 0;
    const replay = new InMemoryInboundReplayRepository();
    const sharedInput = input(client({
      send: async () => {
        sent += 1;
        await sending;
      },
    }), replay);
    const first = processInboundEmail(receivedEvent(), sharedInput);
    await Promise.resolve();
    await expect(processInboundEmail(receivedEvent(), {
      ...sharedInput,
      svixId: "msg_fixture_2",
    })).resolves.toEqual({ action: "in_progress" });
    releaseSend?.();
    await expect(first).resolves.toEqual({ action: "forwarded" });
    await expect(processInboundEmail(receivedEvent(), {
      ...sharedInput,
      svixId: "msg_fixture_3",
    })).resolves.toEqual({ action: "ignored", reason: "duplicate" });
    expect(sent).toBe(1);
  });

  test("releases transient send failures but terminally rejects malformed provider content", async () => {
    const replay = new InMemoryInboundReplayRepository();
    let sends = 0;
    const transient = input(client({
      send: async () => {
        sends += 1;
        if (sends === 1) throw new Error("temporary provider error");
      },
    }), replay);
    await expect(processInboundEmail(receivedEvent(), transient)).rejects.toMatchObject({
      code: "email_forwarding_failed",
    });
    await expect(processInboundEmail(receivedEvent(), transient)).resolves.toEqual({
      action: "forwarded",
    });

    await expect(processInboundEmail(receivedEvent({ email_id: "received-email-2" }), input(client({
      getMetadata: async () => {
        throw new InboundContentRejectedError();
      },
    })))).resolves.toEqual({ action: "ignored", reason: "rejected" });
  });

  test("records terminal ambiguity without releasing after accepted provider sends lose ledger finalization", async () => {
    for (const outcome of ["false", "throw"] as const) {
      const replay = new FinalizationFailureReplay(outcome);
      let sent = 0;
      const request = input(client({
        send: async () => {
          sent += 1;
        },
      }), replay);
      await expect(processInboundEmail(receivedEvent(), request)).rejects.toMatchObject({
        code: "email_forwarding_failed",
      });
      expect(replay.ambiguousCalls).toBe(1);
      expect(replay.releaseCalls).toBe(0);
      await expect(processInboundEmail(receivedEvent(), {
        ...request,
        svixId: `msg_finalization_${outcome}`,
      })).resolves.toEqual({ action: "ignored", reason: "duplicate" });
      expect(sent).toBe(1);
    }
  });

  test("treats malformed and oversized 2xx provider receipts as terminal ambiguity", async () => {
    for (const receipt of ["{not-json", "x".repeat(64 * 1_024 + 1)]) {
      let sends = 0;
      const receivingClient = new ResendReceivingClient(
        `re_${"a".repeat(40)}`,
        async (_url, request) => {
          if (request?.method === "POST") {
            sends += 1;
            return new Response(receipt, { status: 202 });
          }
          return Response.json(resendMetadata("received-email-1"));
        },
      );
      const replay = new AmbiguityTrackingReplay();
      const request = input(receivingClient, replay);
      await expect(processInboundEmail(receivedEvent(), request)).rejects.toMatchObject({
        code: "email_forwarding_failed",
      });
      expect(replay.ambiguousCalls).toBe(1);
      expect(replay.releaseCalls).toBe(0);
      await expect(processInboundEmail(receivedEvent(), {
        ...request,
        svixId: `msg_receipt_${receipt.length}`,
      })).resolves.toEqual({ action: "ignored", reason: "duplicate" });
      expect(sends).toBe(1);
    }
  });

  test("keeps the external subject prefix inside the byte limit", async () => {
    const messages: ForwardMessage[] = [];
    await processInboundEmail(receivedEvent(), input(client({
      getMetadata: async (id) => ({
        attachmentsPresent: false,
        from: "sender@example.test",
        id,
        receivedFor: ["hello@file.cheap"],
        replyTo: [],
        subject: "é".repeat(512),
        to: ["hello@file.cheap"],
      }),
      send: async (message) => {
        messages.push(message);
      },
    })));
    expect(Buffer.byteLength(messages[0]?.subject ?? "", "utf8")).toBeLessThanOrEqual(512);
    expect(messages[0]?.subject.startsWith("[External] ")).toBe(true);
  });
});
