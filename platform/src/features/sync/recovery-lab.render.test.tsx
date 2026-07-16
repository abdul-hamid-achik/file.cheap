import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import { RecoveryLab } from "@/features/sync/recovery-lab";

describe("RecoveryLab initial semantics", () => {
  test("renders a locked, safety-first workflow without exposing a token", () => {
    const html = renderToStaticMarkup(<RecoveryLab storageDriver="local" />);

    expect(html).toContain("Development bearer token");
    expect(html).toContain('type="password"');
    expect(html).toContain("Synthetic, non-sensitive test data only");
    expect(html).toContain('aria-live="polite"');
    expect(html).toContain("Portable recovery drill");
    expect(html).toContain("Unlock the vault to load its catalog");
    expect(html).not.toContain('value="local-development-token"');
  });
});
