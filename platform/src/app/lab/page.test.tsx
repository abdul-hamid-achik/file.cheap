import { describe, expect, test } from "bun:test";

import { metadata } from "@/app/lab/page";

describe("recovery lab page metadata", () => {
  test("keeps the experimental surface out of search results", () => {
    expect(metadata.alternates).toMatchObject({ canonical: "/lab" });
    expect(metadata.robots).toMatchObject({
      follow: false,
      index: false,
      noarchive: true,
      nosnippet: true,
    });
  });
});
