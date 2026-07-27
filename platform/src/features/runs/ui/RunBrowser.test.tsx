import { expect, mock, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import type { RunSummary } from "@/features/runs/contracts";

mock.module("next/navigation", () => ({
  usePathname: () => "/console/runs",
  useRouter: () => ({ push: () => undefined, replace: () => undefined }),
  useSearchParams: () => new URLSearchParams(),
}));

const { RunBrowser } = await import("./RunBrowser");

test("makes an API date window visible and clearable", () => {
  const run: RunSummary = {
    artifactId: "art_00000000000000000001",
    counts: { artifacts: 1, outcomes: 1, steps: 1 },
    createdAt: "2026-07-26T12:00:00.000Z",
    detector: { name: "glyphrun-run", version: "1" },
    evidence: [],
    health: {
      changed: 0,
      declared: 0,
      empty: 0,
      missing: 0,
      present: 0,
      reasons: [],
      state: "ok",
    },
    outcomes: [],
    producer: {
      native_id: "run-1",
      native_schema: "urn:glyphrun.dev:run:v1",
      tool: "glyphrun",
    },
    run: {
      nativeId: "run-1",
      seriesKey: "series-key-000000000001",
      startedAt: "2026-07-26T12:00:00.000Z",
      status: "passed",
    },
    runIndexSha256: "b".repeat(64),
    source: {
      contentType: "application/gzip",
      kind: "glyphrun.run",
      sha256: "a".repeat(64),
      sizeBytes: 100,
    },
    updatedAt: "2026-07-26T12:00:00.000Z",
  };

  const html = renderToStaticMarkup(
    <RunBrowser
      facets={{ health: [], producers: [{ count: 1, value: "glyphrun" }], statuses: [] }}
      filteredTotal={1}
      initialQuery={{
        direction: "next",
        from: "2026-07-25T00:00:00.000Z",
        health: "ok",
        limit: 25,
        q: "checkout",
        to: "2026-07-27T00:00:00.000Z",
      }}
      page={1}
      pageInfo={{
        endCursor: "cursor_next",
        hasNextPage: false,
        hasPreviousPage: false,
        startCursor: "cursor_start",
      }}
      recordedTotal={1}
      runs={[run]}
    />,
  );

  expect(html).toContain("Date window");
  expect(html).toContain("2026-07-25 00:00:00 UTC to 2026-07-27 00:00:00 UTC");
  expect(html).toContain("Clear date window");
  expect(html).toContain('aria-label="Active run filters"');
  expect(html).toContain("Search: checkout · Evidence: ok");
  expect(html).toContain('aria-label="Run results"');
  expect(html).toContain('tabindex="-1"');
});
