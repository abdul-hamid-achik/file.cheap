import { describe, expect, test } from "bun:test";

import { encodeConsoleCatalogCursor } from "@/features/console/catalog/cursor";

import { artifactPageState, runPageState } from "./page-params";

const artifactId = `art_${"a".repeat(24)}`;

describe("console catalog page params", () => {
  test("preserves valid artifact filters and cursor page state", () => {
    const cursor = encodeConsoleCatalogCursor("artifacts", {
      id: artifactId,
      time: new Date("2026-07-26T12:00:00.000Z"),
    });
    expect(artifactPageState({
      cursor,
      direction: "next",
      groupBy: "kind",
      kind: "chalupa.log-chunk",
      limit: "25",
      page: "3",
      producer: "chalupa",
      q: "session",
    })).toMatchObject({
      groupBy: "kind",
      page: 3,
      query: { cursor, kind: "chalupa.log-chunk", limit: 25, producer: "chalupa", q: "session" },
    });
  });

  test("drops a cursor from the wrong catalog scope", () => {
    const cursor = encodeConsoleCatalogCursor("artifacts", {
      id: artifactId,
      time: new Date("2026-07-26T12:00:00.000Z"),
    });
    expect(runPageState({ cursor, direction: "next", page: "9" })).toMatchObject({
      page: 1,
      query: { cursor: undefined, direction: "next" },
    });
  });

  test("fails safe to defaults when a query value is invalid", () => {
    expect(artifactPageState({ limit: "500", page: "2", producer: "!" })).toMatchObject({
      page: 1,
      query: { direction: "next", limit: 25 },
    });
  });
});
