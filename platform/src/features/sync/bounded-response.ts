export async function readExactResponseBytes(
  response: Response,
  expectedSizeBytes: number,
): Promise<ArrayBuffer> {
  if (!Number.isSafeInteger(expectedSizeBytes) || expectedSizeBytes < 0) {
    await cancelResponseBody(response, "Recovery expectation is invalid");
    throw new Error("Expected download size is invalid.");
  }

  const advertisedLength = response.headers.get("content-length");
  if (advertisedLength !== null) {
    const parsedLength = Number(advertisedLength);
    if (
      !/^\d+$/.test(advertisedLength) ||
      !Number.isSafeInteger(parsedLength) ||
      parsedLength !== expectedSizeBytes
    ) {
      await cancelResponseBody(response, "Recovery Content-Length is invalid");
      throw new Error(
        "The recovery response Content-Length does not match the recovery card.",
      );
    }
  }

  if (!response.body) {
    if (expectedSizeBytes === 0) return new ArrayBuffer(0);
    throw new Error("The recovery response did not include a body.");
  }

  const bytes = new Uint8Array(expectedSizeBytes);
  const reader = response.body.getReader();
  let receivedBytes = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (receivedBytes + value.byteLength > expectedSizeBytes) {
        await reader.cancel("Recovery response exceeded its declared size").catch(
          () => undefined,
        );
        throw new Error(
          "The recovery response exceeded the size in the recovery card.",
        );
      }
      bytes.set(value, receivedBytes);
      receivedBytes += value.byteLength;
    }
  } finally {
    reader.releaseLock();
  }

  if (receivedBytes !== expectedSizeBytes) {
    throw new Error(
      "The recovery response ended before the size in the recovery card.",
    );
  }
  return bytes.buffer;
}

async function cancelResponseBody(
  response: Response,
  reason: string,
): Promise<void> {
  if (!response.body || response.body.locked) return;
  await response.body.cancel(reason).catch(() => undefined);
}
