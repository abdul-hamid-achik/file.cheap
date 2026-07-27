import { describe, expect, test } from "bun:test";

import {
  catalogDrawerCloseMode,
  catalogDrawerHref,
  resolveCatalogDrawer,
} from "./catalog-drawer-url";

const items = [{ id: "art_one" }, { id: "art_two" }];

describe("catalog drawer URL state", () => {
  test("opens only an exact item present on the current page", () => {
    expect(resolveCatalogDrawer(
      new URLSearchParams("q=trace&artifact=art_two"),
      "artifact",
      items,
      (item) => item.id,
    )).toEqual({ item: items[1], requestedId: "art_two", shouldClean: false });
  });

  test("marks absent, blank, and duplicate selections for one-time cleanup", () => {
    for (const query of [
      "artifact=art_missing",
      "artifact=",
      "artifact=art_one&artifact=art_two",
    ]) {
      expect(resolveCatalogDrawer(
        new URLSearchParams(query),
        "artifact",
        items,
        (item) => item.id,
      )).toMatchObject({ item: null, shouldClean: true });
    }
    expect(resolveCatalogDrawer(
      new URLSearchParams("q=trace"),
      "artifact",
      items,
      (item) => item.id,
    )).toEqual({ item: null, requestedId: null, shouldClean: false });
  });

  test("preserves filters and cursor state when opening and closing", () => {
    const source = new URLSearchParams("q=trace&producer=glyphrun&cursor=opaque%2Fcursor&page=3");
    const open = catalogDrawerHref("/console", source, "artifact", "art_two");
    expect(open).toBe("/console?q=trace&producer=glyphrun&cursor=opaque%2Fcursor&page=3&artifact=art_two");
    expect(catalogDrawerHref(
      "/console",
      new URL(open, "https://file.cheap").searchParams,
      "artifact",
    )).toBe("/console?q=trace&producer=glyphrun&cursor=opaque%2Fcursor&page=3");
  });

  test("uses Back only for the exact history entry created by the drawer", () => {
    const href = "/console?q=trace&artifact=art_two";
    expect(catalogDrawerCloseMode(href, href)).toBe("back");
    expect(catalogDrawerCloseMode(href, null)).toBe("replace");
    expect(catalogDrawerCloseMode(href, "/console?artifact=art_two")).toBe("replace");
  });
});
