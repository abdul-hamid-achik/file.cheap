import { timingSafeEqual } from "node:crypto";

import { getConfig } from "@/shared/config/env";
import { PlatformError } from "@/shared/errors/platform-error";

export function requireApiToken(request: Request): void {
  const authorization = request.headers.get("authorization");
  const expected = `Bearer ${getConfig().apiToken}`;

  if (!authorization || !constantTimeEqual(authorization, expected)) {
    throw new PlatformError({
      code: "unauthorized",
      detail: "A valid bearer token is required.",
      status: 401,
      title: "Unauthorized",
    });
  }
}

function constantTimeEqual(received: string, expected: string): boolean {
  const receivedBytes = Buffer.from(received);
  const expectedBytes = Buffer.from(expected);

  if (receivedBytes.length !== expectedBytes.length) {
    return false;
  }

  return timingSafeEqual(receivedBytes, expectedBytes);
}
