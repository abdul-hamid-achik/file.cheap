import { describe, expect, mock, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import type { ConsoleArtifact } from "./artifact-types";

mock.module("next/navigation", () => ({
  usePathname: () => "/console",
  useRouter: () => ({ push: () => undefined, replace: () => undefined }),
  useSearchParams: () => new URLSearchParams(),
}));

const { ArtifactBrowser } = await import("./ArtifactBrowser");

describe("artifact browser", () => {
  test("renders a bounded first page for large artifact collections", () => {
    const artifacts = Array.from({ length: 10 }, (_, index): ConsoleArtifact => ({
      availability: "cloud-ready",
      id: `artifact-${index + 1}`,
      integrity: "server-sha256",
      kind: "log",
      label: `Artifact ${index + 1}`,
      producer: { tool: "chalupa" },
      state: "committed",
    }));

    const html = renderToStaticMarkup(
      <ArtifactBrowser
        artifacts={artifacts}
        facets={{ kinds: [{ count: 17, value: "log" }], producers: [{ count: 17, value: "chalupa" }] }}
        filteredTotal={17}
        initialGroupBy="producer"
        initialQuery={{ direction: "next", limit: 10 }}
        page={1}
        pageInfo={{
          endCursor: "cursor_next",
          hasNextPage: true,
          hasPreviousPage: false,
          startCursor: "cursor_start",
        }}
        recordedTotal={17}
      />,
    );

    expect(html).toContain("Artifact 10");
    expect(html).toContain("<strong>10</strong> on this page · 17 total");
    expect(html).toContain("10 on this page · 17 matching");
    expect(html).toContain("chalupa (17)");
    expect(html).toContain('aria-label="Artifact filters"');
    expect(html).toContain('aria-label="Artifact results"');
    expect(html).toContain('tabindex="-1"');
  });

  test("surfaces active filters with one clear action", () => {
    const html = renderToStaticMarkup(
      <ArtifactBrowser
        artifacts={[]}
        facets={{ kinds: [], producers: [] }}
        filteredTotal={0}
        initialGroupBy="producer"
        initialQuery={{ direction: "next", kind: "log", limit: 10, producer: "chalupa", q: "run-42" }}
        page={1}
        pageInfo={{ endCursor: null, hasNextPage: false, hasPreviousPage: false, startCursor: null }}
        recordedTotal={17}
      />,
    );

    expect(html).toContain('aria-label="Active artifact filters"');
    expect(html).toContain("Search: run-42 · Producer: chalupa · Kind: log");
    expect(html).toContain("Clear filters");
  });

  test("offers a deterministic recovery when a cursor page becomes empty", () => {
    const html = renderToStaticMarkup(
      <ArtifactBrowser
        artifacts={[]}
        facets={{ kinds: [], producers: [] }}
        filteredTotal={17}
        initialGroupBy="producer"
        initialQuery={{ cursor: "cursor_stale", direction: "next", limit: 10 }}
        page={2}
        pageInfo={{
          endCursor: null,
          hasNextPage: false,
          hasPreviousPage: true,
          startCursor: null,
        }}
        recordedTotal={17}
      />,
    );

    expect(html).toContain("This cursor page is no longer available");
    expect(html).toContain("Return to first page");
    expect(html).toContain("<strong>0</strong> on this page · 17 total");
  });
});
