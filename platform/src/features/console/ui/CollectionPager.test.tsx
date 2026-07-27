import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import { CollectionPager } from "./CollectionPager";

describe("console collection pager", () => {
  test("announces cursor-safe counts and disables the terminal direction", () => {
    const html = renderToStaticMarkup(
      <CollectionPager
        currentPage={2}
        hasNextPage={false}
        hasPreviousPage
        itemLabel="artifacts"
        onNextPage={() => undefined}
        onPageSizeChange={() => undefined}
        onPreviousPage={() => undefined}
        pageSize={10}
        totalItems={17}
        visibleItems={7}
      />,
    );

    expect(html).toContain("<strong>7</strong> on this page · 17 total");
    expect(html).toContain("Page 2");
    expect(html).toContain('aria-label="Previous artifacts page"');
    expect(html).toContain('aria-label="Next artifacts page" disabled=""');
  });
});
