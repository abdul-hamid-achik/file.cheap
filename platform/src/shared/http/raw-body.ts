import { PlatformError } from "@/shared/errors/platform-error";

function payloadTooLarge(maxBytes: number): PlatformError {
  return new PlatformError({
    code: "payload_too_large",
    detail: `The request body is limited to ${maxBytes} bytes.`,
    status: 413,
    title: "Payload too large",
  });
}

export async function readLimitedUtf8Body(
  request: Request,
  maxBytes: number,
): Promise<string> {
  const advertisedLength = request.headers.get("content-length");
  if (advertisedLength !== null) {
    const length = Number(advertisedLength);
    if (Number.isFinite(length) && length > maxBytes) {
      throw payloadTooLarge(maxBytes);
    }
  }

  if (!request.body) return "";
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let sizeBytes = 0;

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      sizeBytes += value.byteLength;
      if (sizeBytes > maxBytes) {
        await reader.cancel("Request body exceeded its limit").catch(() => undefined);
        throw payloadTooLarge(maxBytes);
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(sizeBytes);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }

  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(body);
  } catch {
    throw new PlatformError({
      code: "invalid_body_encoding",
      detail: "The request body must be valid UTF-8.",
      status: 400,
      title: "Invalid body encoding",
    });
  }
}
