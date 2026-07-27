import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import { ConsoleShell } from "./ConsoleShell";

test("offers a keyboard skip link to the focusable main console region", () => {
  const html = renderToStaticMarkup(
    <ConsoleShell
      navigation={[{ current: true, href: "/console", label: "Artifacts" }]}
      sessionLabel="owner@example.com"
    >
      <p>Catalog</p>
    </ConsoleShell>,
  );

  expect(html).toContain('href="#main-content"');
  expect(html).toContain('>Skip to main content</a>');
  expect(html).toContain('<main id="main-content" tabindex="-1">');
  expect(html.indexOf('href="#main-content"')).toBeLessThan(html.indexOf('aria-label="Console navigation"'));
});
