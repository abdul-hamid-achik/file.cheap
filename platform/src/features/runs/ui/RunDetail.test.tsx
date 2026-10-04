import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import type { RunSummary } from "@/features/runs/contracts";

import { RunDetail } from "./RunDetail";

const run: RunSummary = {
  artifactId: "art_00000000000000000001",
  counts: { artifacts: 1, outcomes: 0, steps: 3 },
  createdAt: "2026-07-26T12:00:00.000Z",
  detector: { name: "cairntrace-run", version: "1" },
  evidence: [],
  health: { changed: 0, declared: 0, empty: 0, missing: 0, present: 0, reasons: [], state: "ok" },
  outcomes: [],
  producer: { native_id: "run-1", native_schema: "urn:cairntrace.dev:run:v1", tool: "cairntrace" },
  run: { nativeId: "run-1", seriesKey: "series-key-000000000001", startedAt: "2026-07-26T12:00:00.000Z", status: "passed" },
  runIndexSha256: "b".repeat(64),
  source: { contentType: "application/gzip", kind: "cairntrace.run", sha256: "a".repeat(64), sizeBytes: 100 },
  updatedAt: "2026-07-26T12:00:00.000Z",
};

test("shows the exact restore command with a copy button on the summary tab", () => {
  const html = renderToStaticMarkup(<RunDetail onClose={() => undefined} run={run} />);
  expect(html).toContain("Restore this run");
  expect(html).toContain(
    "fcheap pull art_00000000000000000001 --output ./run-1.tar.gz &amp;&amp; mkdir -p ./run-1 &amp;&amp; tar -xzf ./run-1.tar.gz -C ./run-1",
  );
  expect(html).toContain('aria-label="Copy run restore command"');
});

test("renders nothing without a selected run", () => {
  expect(renderToStaticMarkup(<RunDetail onClose={() => undefined} run={null} />)).toBe("");
});
