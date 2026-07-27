import { describe, expect, test } from "bun:test";

import { parsePostgresTestTarget } from "./postgres-test-safety";

describe("Postgres integration-test safety", () => {
  test("accepts a loopback database whose name is explicitly test-only", () => {
    expect(
      parsePostgresTestTarget(
        "postgresql://filecheap:secret@127.0.0.1:5432/filecheap_test",
        { allowRemote: false },
      ),
    ).toMatchObject({
      databaseName: "filecheap_test",
      host: "127.0.0.1",
      remote: false,
    });
  });

  test("rejects a database name that could be production", () => {
    expect(() =>
      parsePostgresTestTarget(
        "postgresql://filecheap:secret@127.0.0.1:5432/filecheap",
        { allowRemote: false },
      ),
    ).toThrow("unmistakable test database");
  });

  test("requires a second explicit opt-in for remote test databases", () => {
    const databaseUrl =
      "postgresql://filecheap:secret@ep-example.neon.tech/filecheap_test";
    expect(() =>
      parsePostgresTestTarget(databaseUrl, { allowRemote: false }),
    ).toThrow("FILECHEAP_ALLOW_REMOTE_TEST_DATABASE=true");
    expect(
      parsePostgresTestTarget(databaseUrl, { allowRemote: true }),
    ).toMatchObject({ remote: true });
  });

  test("rejects non-Postgres URLs before any connection attempt", () => {
    expect(() =>
      parsePostgresTestTarget("https://localhost/filecheap_test", {
        allowRemote: false,
      }),
    ).toThrow("postgres or postgresql protocol");
  });
});
