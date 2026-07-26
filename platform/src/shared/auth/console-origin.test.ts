import { expect, test } from "bun:test";

import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";

test("accepts same-origin browser mutations and rejects cross-site cookies", () => {
  expect(() => assertConsoleMutationOrigin(new Request("https://file.cheap/api/console/artifacts/a", {
    headers: { origin: "https://file.cheap", "sec-fetch-site": "same-origin" },
    method: "DELETE",
  }))).not.toThrow();
  expect(() => assertConsoleMutationOrigin(new Request("https://file.cheap/api/console/artifacts/a", {
    headers: { origin: "https://attacker.example", "sec-fetch-site": "cross-site" },
    method: "DELETE",
  }))).toThrow();
});

test("does not apply browser CSRF checks to non-ambient bearer requests", () => {
  expect(() => assertConsoleMutationOrigin(new Request("https://file.cheap/api/console/artifacts/a", {
    headers: { authorization: "Bearer invalid-but-not-ambient" },
    method: "DELETE",
  }))).not.toThrow();
});
