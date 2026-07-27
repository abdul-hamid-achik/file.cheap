export type CatalogDrawerParam = "artifact" | "run";

export interface CatalogDrawerResolution<Item> {
  item: Item | null;
  requestedId: string | null;
  shouldClean: boolean;
}

/** Resolve one URL-selected drawer only when its exact item is on this page. */
export function resolveCatalogDrawer<Item>(
  searchParams: URLSearchParams,
  param: CatalogDrawerParam,
  items: readonly Item[],
  itemId: (item: Item) => string,
): CatalogDrawerResolution<Item> {
  const values = searchParams.getAll(param);
  if (values.length === 0) {
    return { item: null, requestedId: null, shouldClean: false };
  }
  const requestedId = values.length === 1 ? values[0]?.trim() || null : null;
  if (!requestedId) {
    return { item: null, requestedId, shouldClean: true };
  }
  const item = items.find((candidate) => itemId(candidate) === requestedId) ?? null;
  return { item, requestedId, shouldClean: item === null };
}

/** Preserve catalog filters and pagination while changing only drawer state. */
export function catalogDrawerHref(
  pathname: string,
  searchParams: URLSearchParams,
  param: CatalogDrawerParam,
  itemId?: string,
): string {
  const next = new URLSearchParams(searchParams.toString());
  next.delete(param);
  if (itemId) next.set(param, itemId);
  const query = next.toString();
  return query ? `${pathname}?${query}` : pathname;
}

export function catalogDrawerCloseMode(
  currentHref: string,
  createdDrawerHref: string | null,
): "back" | "replace" {
  return createdDrawerHref === currentHref ? "back" : "replace";
}
