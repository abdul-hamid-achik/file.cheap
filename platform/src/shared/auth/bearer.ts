import { timingSafeEqual } from "node:crypto";

import { getConfig } from "@/shared/config/env";
import { PlatformError } from "@/shared/errors/platform-error";

export function requireApiToken(
  request: Request,
  expectedToken = getConfig().apiToken,
): void {
  const authorization = request.headers.get("authorization");
  const credential = authorization
    ? /^[\t ]*Bearer[\t ]+([^\t ]+)[\t ]*$/i.exec(authorization)?.[1]
    : undefined;

  if (!credential || !constantTimeEqual(credential, expectedToken)) {
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
