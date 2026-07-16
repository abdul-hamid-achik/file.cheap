import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import { RecoveryLab, responseError } from "@/features/sync/recovery-lab";

describe("RecoveryLab initial semantics", () => {
  test("renders a locked, safety-first workflow without exposing a token", () => {
    const html = renderToStaticMarkup(<RecoveryLab storageDriver="local" />);

    expect(html).toContain("Development bearer token");
    expect(html).toContain('type="password"');
    expect(html).toContain("Synthetic, non-sensitive test data only");
    expect(html).toContain('aria-live="polite"');
    expect(html).toContain('aria-busy="false"');
    expect(html).toContain("Portable recovery drill");
    expect(html).toContain("Protocol v1 permanently binds this ID");
    expect(html).toContain("Upload + commit");
    expect(html).toContain("Unlock the vault to load its catalog");
    expect(html).not.toContain('value="local-development-token"');
  });

  test("fails safely when a problem response contains render-unsafe fields", async () => {
    const error = await responseError(
      Response.json(
        { detail: { unsafe: true }, requestId: { unsafe: true } },
        {
          headers: { "x-request-id": "safe-header-request-id" },
          status: 500,
        },
      ),
      "control",
    );

    expect(error.message).toBe("Request failed (500)");
    expect(error.requestId).toBe("safe-header-request-id");
    expect(error.status).toBe(500);
  });
});
