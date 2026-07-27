import { Buffer } from "node:buffer";

import { artifactIdSchema } from "@/features/artifacts/contracts";
import { consoleCatalogCursorSchema } from "@/features/console/catalog/contracts";
import { PlatformError } from "@/shared/errors/platform-error";

export type ConsoleCatalogScope = "artifacts" | "runs";
export type ConsoleCatalogCursor = Readonly<{ id: string; time: Date }>;

export function encodeConsoleCatalogCursor(
  scope: ConsoleCatalogScope,
  value: ConsoleCatalogCursor,
): string {
  return Buffer.from(
    JSON.stringify([1, scope, value.time.toISOString(), value.id]),
    "utf8",
  ).toString("base64url");
}

export function decodeConsoleCatalogCursor(
  encoded: string,
  expectedScope: ConsoleCatalogScope,
): ConsoleCatalogCursor {
  try {
    consoleCatalogCursorSchema.parse(encoded);
    const value = JSON.parse(
      Buffer.from(encoded, "base64url").toString("utf8"),
    ) as unknown;
    if (
      !Array.isArray(value) ||
      value.length !== 4 ||
      value[0] !== 1 ||
      value[1] !== expectedScope ||
      typeof value[2] !== "string" ||
      typeof value[3] !== "string"
    ) {
      throw new Error("invalid cursor shape");
    }
    const time = new Date(value[2]);
    artifactIdSchema.parse(value[3]);
    if (
      Number.isNaN(time.getTime()) ||
      time.toISOString() !== value[2]
    ) {
      throw new Error("invalid cursor values");
    }
    return { id: value[3], time };
  } catch {
    throw new PlatformError({
      code: "invalid_cursor",
      detail: "The console catalog cursor is invalid.",
      status: 422,
      title: "Invalid cursor",
    });
  }
}
