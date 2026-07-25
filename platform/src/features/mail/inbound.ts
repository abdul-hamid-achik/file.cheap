import { z } from "zod";

import {
  AcceptedProviderReceiptError,
  InboundContentRejectedError,
} from "@/features/mail/errors";
import {
  replayDigest,
  type InboundReplayRepository,
} from "@/features/mail/replay-repository";
import { PlatformError } from "@/shared/errors/platform-error";

export const FILECHEAP_INBOUND_EMAIL = "hello@file.cheap";
export const FILECHEAP_FORWARDING_EMAIL = "inbox@file.cheap";
export const FILECHEAP_FORWARDING_FROM =
  "file.cheap Inbox <inbox@file.cheap>";

const MAX_EMAIL_ID_LENGTH = 256;
const MAX_SUBJECT_LENGTH = 512;

const receivedEventSchema = z.object({
  data: z.object({
    email_id: z.string().min(1).max(MAX_EMAIL_ID_LENGTH).regex(/^[A-Za-z0-9_-]+$/u),
    from: z.string().min(3).max(512),
    received_for: z.array(z.string().min(3).max(320)).min(1).max(64),
  }),
  type: z.literal("email.received"),
});

const emailAddressSchema = z.string().email().max(320);

export interface ReceivedEmailMetadata {
  attachmentsPresent: boolean;
  from: string;
  html?: string;
  id: string;
  receivedFor: string[];
  replyTo: string[];
  subject: string;
  text?: string;
  to: string[];
}

export interface ForwardMessage {
  from: string;
  html?: string;
  idempotencyKey: string;
  replyTo: string;
  subject: string;
  text?: string;
  to: string;
}

export interface InboundMailClient {
  getMetadata(emailId: string): Promise<ReceivedEmailMetadata>;
  send(message: ForwardMessage): Promise<void>;
}

export type InboundEmailResult =
  | { action: "ignored"; reason: "duplicate" | "event_type" | "loop" | "recipient" | "rejected" }
  | { action: "forwarded" }
  | { action: "in_progress" };

function invalidEvent(): PlatformError {
  return new PlatformError({
    code: "invalid_inbound_email_event",
    detail: "The signed email event is invalid.",
    status: 400,
    title: "Invalid inbound email event",
  });
}

function forwardingFailed(): PlatformError {
  return new PlatformError({
    code: "email_forwarding_failed",
    detail: "Inbound email forwarding could not be completed.",
    retryAfterSeconds: 1,
    status: 502,
    title: "Email forwarding failed",
  });
}

class AcceptedProviderLedgerError extends Error {
  constructor() {
    super("The provider accepted forwarding before replay finalization failed");
    this.name = "AcceptedProviderLedgerError";
  }
}

function normalizeAddress(value: string): string | null {
  const bracketed = /<\s*([^<>]+)\s*>\s*$/u.exec(value)?.[1];
  const candidate = (bracketed ?? value).trim().toLowerCase();
  return emailAddressSchema.safeParse(candidate).success ? candidate : null;
}

function replyAddress(metadata: ReceivedEmailMetadata): string | null {
  const candidates = [
    ...metadata.replyTo.map(normalizeAddress),
    normalizeAddress(metadata.from),
  ];
  return candidates.find((value): value is string => value !== null) ?? null;
}

export async function processInboundEmail(
  event: unknown,
  input: {
    client: InboundMailClient;
    forwardTo: string;
    now?: () => Date;
    replay: InboundReplayRepository;
    svixId: string;
  },
): Promise<InboundEmailResult> {
  if (
    typeof event === "object" &&
    event !== null &&
    "type" in event &&
    event.type !== "email.received"
  ) {
    return { action: "ignored", reason: "event_type" };
  }
  const parsedEvent = receivedEventSchema.safeParse(event);
  if (!parsedEvent.success) throw invalidEvent();

  const eventRecipients = parsedEvent.data.data.received_for.map(normalizeAddress);
  if (
    eventRecipients.length !== 1 ||
    eventRecipients[0] !== FILECHEAP_INBOUND_EMAIL
  ) {
    return { action: "ignored", reason: "recipient" };
  }
  const eventSender = normalizeAddress(parsedEvent.data.data.from);
  if (
    !eventSender ||
    [
      FILECHEAP_INBOUND_EMAIL,
      FILECHEAP_FORWARDING_EMAIL,
    ].includes(eventSender)
  ) {
    return { action: "ignored", reason: "loop" };
  }
  const forwardTo = normalizeAddress(input.forwardTo);
  if (!forwardTo) throw invalidEvent();
  if (eventSender === forwardTo) return { action: "ignored", reason: "loop" };
  const now = input.now ?? (() => new Date());
  const emailId = parsedEvent.data.data.email_id;
  const emailIdSha256 = replayDigest(emailId);
  const replay = await input.replay.claim({
    emailIdSha256,
    now: now(),
    svixIdSha256: replayDigest(input.svixId),
  });
  if (replay.state === "duplicate") {
    return { action: "ignored", reason: "duplicate" };
  }
  if (replay.state === "in_progress") return { action: "in_progress" };

  try {
    const metadata = await input.client.getMetadata(emailId);
    const recipients = metadata.receivedFor.map(normalizeAddress);
    const deliveredTo = metadata.to.map(normalizeAddress);
    const sender = normalizeAddress(metadata.from);
    if (
      metadata.id !== emailId ||
      recipients.length !== 1 ||
      recipients[0] !== FILECHEAP_INBOUND_EMAIL ||
      deliveredTo.length !== 1 ||
      deliveredTo[0] !== FILECHEAP_INBOUND_EMAIL ||
      !sender
    ) {
      await input.replay.markIgnored(emailIdSha256, replay.leaseToken, now());
      return { action: "ignored", reason: "recipient" };
    }

    const replyTo = replyAddress(metadata);
    if (
      !replyTo ||
      [
        FILECHEAP_INBOUND_EMAIL,
        FILECHEAP_FORWARDING_EMAIL,
        forwardTo,
      ].includes(sender) ||
      [
        FILECHEAP_INBOUND_EMAIL,
        FILECHEAP_FORWARDING_EMAIL,
        forwardTo,
      ].includes(replyTo)
    ) {
      await input.replay.markIgnored(emailIdSha256, replay.leaseToken, now());
      return { action: "ignored", reason: "loop" };
    }

    try {
      await input.client.send({
        from: FILECHEAP_FORWARDING_FROM,
        html: forwardedHtml(metadata.html, metadata.attachmentsPresent),
        idempotencyKey: `inbound-forward/filecheap/${emailIdSha256}`,
        replyTo,
        subject: forwardedSubject(metadata.subject),
        text: forwardText(metadata.text, metadata.attachmentsPresent),
        to: forwardTo,
      });
    } catch (error) {
      if (error instanceof AcceptedProviderReceiptError) {
        await input.replay.markAmbiguous(emailIdSha256, replay.leaseToken, now())
          .catch(() => undefined);
        throw new AcceptedProviderLedgerError();
      }
      throw forwardingFailed();
    }
    try {
      if (!await input.replay.markForwarded(
        emailIdSha256,
        replay.leaseToken,
        now(),
      )) {
        throw forwardingFailed();
      }
    } catch {
      await input.replay.markAmbiguous(emailIdSha256, replay.leaseToken, now())
        .catch(() => undefined);
      throw new AcceptedProviderLedgerError();
    }
    return { action: "forwarded" };
  } catch (error) {
    if (error instanceof InboundContentRejectedError) {
      await input.replay.markRejected(emailIdSha256, replay.leaseToken, now())
        .catch(() => undefined);
      return { action: "ignored", reason: "rejected" };
    }
    if (error instanceof AcceptedProviderLedgerError) throw forwardingFailed();
    await input.replay.release(emailIdSha256, replay.leaseToken, now())
      .catch(() => undefined);
    if (error instanceof PlatformError) throw error;
    throw forwardingFailed();
  }
}

function withAttachmentNotice(
  body: string | undefined,
  format: "html" | "text",
  attachmentsPresent: boolean,
): string | undefined {
  if (!attachmentsPresent) return body;
  const notice = format === "html"
    ? "<p><em>Attachments were omitted. Use file.cheap for files.</em></p>"
    : "\n\n[Attachments were omitted. Use file.cheap for files.]";
  return `${body ?? ""}${notice}`;
}

function forwardText(
  body: string | undefined,
  attachmentsPresent: boolean,
): string {
  const rendered = body
    ? withAttachmentNotice(body, "text", attachmentsPresent)!
    : attachmentsPresent
    ? "[Attachments were omitted. Use file.cheap for files.]"
    : "[This message has no renderable body.]";
  return `[External email]\n\n${rendered}`;
}

function forwardedHtml(
  body: string | undefined,
  attachmentsPresent: boolean,
): string | undefined {
  if (!body) return undefined;
  // CID attachments are deliberately not fetched, so remove only those image
  // elements rather than claiming the untrusted HTML is sanitized.
  const withoutCidImages = body.replace(
    /<img\b[^>]*\bsrc\s*=\s*(["'])cid:[^"']*\1[^>]*>/giu,
    "",
  );
  return `<p><strong>External email</strong></p>${withAttachmentNotice(
    withoutCidImages,
    "html",
    attachmentsPresent,
  ) ?? ""}`;
}

function forwardedSubject(subject: string): string {
  const prefix = "[External] ";
  return truncateUtf8(`${prefix}${subject || "Forwarded message"}`,
    MAX_SUBJECT_LENGTH,
  );
}

function truncateUtf8(value: string, maxBytes: number): string {
  const bytes = Buffer.from(value, "utf8");
  if (bytes.byteLength <= maxBytes) return value;
  const decoder = new TextDecoder("utf-8", { fatal: true });
  for (let length = maxBytes; length >= 0; length -= 1) {
    try {
      return decoder.decode(bytes.subarray(0, length));
    } catch {
      // A partial multibyte code point is removed, never replaced.
    }
  }
  return "";
}
