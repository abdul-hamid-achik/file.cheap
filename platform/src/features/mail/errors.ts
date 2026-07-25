export class InboundContentRejectedError extends Error {
  constructor() {
    super("Inbound email content exceeded a forwarding policy limit");
    this.name = "InboundContentRejectedError";
  }
}

// A 2xx send response proves Resend accepted the request even when its receipt
// cannot be safely interpreted. Retrying it after idempotency retention could
// create a second message, so the replay must become terminally ambiguous.
export class AcceptedProviderReceiptError extends Error {
  constructor() {
    super("The provider accepted forwarding but returned an unreadable receipt");
    this.name = "AcceptedProviderReceiptError";
  }
}
