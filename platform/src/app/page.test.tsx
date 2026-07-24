import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import HomePage, { metadata } from "@/app/page";

describe("public homepage", () => {
  test("renders the local product without runtime platform configuration", () => {
    const html = renderToStaticMarkup(<HomePage />);

    expect(html).toContain("Keep the files your agents create");
    expect(html).toContain('href="/guide/getting-started"');
    expect(html).toContain('href="/integrations/local-artifact-references"');
    expect(html).toContain("Fifteen typed local tools");
    expect(html).toContain('href="#main-content"');
    expect(html).toContain('id="main-content"');
    expect(html).toContain('class="navIntegrationLink"');
    expect(html).toContain('class="navOptionalLink" href="/guide/"');
    expect(html).toContain("Cairntrace or Glyphrun");
    expect(html).toContain("ArtifactRefV1");
    expect(html).toContain("Chalupa");
    expect(html).toContain("Once separately deployed");
    expect(html).toContain("not a sync service");
    expect(html).not.toContain("Development bearer token");
    expect(html).not.toContain("$15");
    expect(html).not.toContain("Remote Vault Beta");
  });

  test("is explicitly indexable at the canonical origin", () => {
    expect(metadata.alternates).toMatchObject({ canonical: "/" });
    expect(metadata.robots).toMatchObject({
      follow: true,
      index: true,
    });
    expect(metadata.openGraph?.images).toBeArray();
    expect(metadata.twitter).toMatchObject({
      card: "summary_large_image",
      images: ["/og.png"],
    });
  });
});
