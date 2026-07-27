import { describe, expect, test } from "bun:test";

import {
  assertExactConsoleOwner,
  ConsoleOwnerCheckError,
  parseConsoleOwnerCheckInput,
} from "./check-console-owner-core";

const owner = {
  FILECHEAP_OWNER_ACCOUNT_ID: "acc_owner123",
  FILECHEAP_OWNER_EMAIL: "Owner@Example.com",
};

describe("parseConsoleOwnerCheckInput", () => {
  test("prefers the direct migration connection and normalizes the owner email", () => {
    const input = parseConsoleOwnerCheckInput({
      ...owner,
      MIGRATIONS_DATABASE_URL: "postgresql://secret@ep-direct.example/filecheap",
      CONSOLE_OWNER_CHECK_DATABASE_URL:
        "postgresql://other-secret@ep-fallback.example/filecheap",
    });

    expect(input).toEqual({
      ownerAccountId: "acc_owner123",
      ownerEmail: "owner@example.com",
      databaseEnvironmentVariable: "MIGRATIONS_DATABASE_URL",
      databaseUrl: "postgresql://secret@ep-direct.example/filecheap",
    });
  });

  test("accepts the dedicated read-only fallback outside Vercel", () => {
    const input = parseConsoleOwnerCheckInput({
      ...owner,
      CONSOLE_OWNER_CHECK_DATABASE_URL:
        "postgres://secret@ep-read-only.example/filecheap",
    });

    expect(input.databaseEnvironmentVariable).toBe(
      "CONSOLE_OWNER_CHECK_DATABASE_URL",
    );
  });

  test.each([
    [{ ...owner }, "missing connection"],
    [
      { ...owner, MIGRATIONS_DATABASE_URL: "not-a-url" },
      "malformed connection",
    ],
    [
      {
        ...owner,
        MIGRATIONS_DATABASE_URL:
          "postgresql://secret@ep-test-pooler.example/filecheap",
      },
      "pooled migration connection",
    ],
    [
      {
        ...owner,
        CONSOLE_OWNER_CHECK_DATABASE_URL:
          "postgresql://secret@ep-read-only.example/filecheap",
        VERCEL: "1",
      },
      "Vercel execution",
    ],
  ])("rejects %s without exposing configured values", (environment) => {
    let thrown: unknown;
    try {
      parseConsoleOwnerCheckInput(environment);
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(ConsoleOwnerCheckError);
    const message = String((thrown as Error).message);
    expect(message).not.toContain("secret");
    expect(message).not.toContain("Owner@Example.com");
  });
});

describe("assertExactConsoleOwner", () => {
  const expected = {
    ownerAccountId: "acc_owner123",
    ownerEmail: "owner@example.com",
  };

  test("accepts exactly one exact row", () => {
    expect(() =>
      assertExactConsoleOwner(expected, [
        { id: "acc_owner123", email: "owner@example.com" },
      ]),
    ).not.toThrow();
  });

  test.each([
    [[], "missing row"],
    [
      [{ id: "acc_other123", email: "owner@example.com" }],
      "mismatched account id",
    ],
    [
      [{ id: "acc_owner123", email: "other@example.com" }],
      "mismatched email",
    ],
    [
      [
        { id: "acc_owner123", email: "other@example.com" },
        { id: "acc_other123", email: "owner@example.com" },
      ],
      "split owner identity",
    ],
  ])("fails closed for %s", (candidates) => {
    expect(() => assertExactConsoleOwner(expected, candidates)).toThrow(
      ConsoleOwnerCheckError,
    );
  });
});
