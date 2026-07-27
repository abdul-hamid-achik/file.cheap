import styles from "./collection-pager.module.css";

interface CollectionPagerProps {
  busy?: boolean;
  currentPage: number;
  hasNextPage: boolean;
  hasPreviousPage: boolean;
  itemLabel: string;
  onNextPage: () => void;
  onPageSizeChange: (pageSize: number) => void;
  onPreviousPage: () => void;
  pageSize: number;
  pageSizeOptions?: readonly number[];
  totalItems: number;
  visibleItems: number;
}

/** Compact cursor navigation for owner-scoped console collections. */
export function CollectionPager({
  busy = false,
  currentPage,
  hasNextPage,
  hasPreviousPage,
  itemLabel,
  onNextPage,
  onPageSizeChange,
  onPreviousPage,
  pageSize,
  pageSizeOptions = [10, 25, 50],
  totalItems,
  visibleItems,
}: CollectionPagerProps) {
  const safePage = Math.max(currentPage, 1);

  return (
    <nav aria-busy={busy} aria-label={`${itemLabel} pagination`} className={styles.pager}>
      <p className={styles.summary}>
        <strong>{visibleItems}</strong> on this page · {totalItems} total
      </p>
      <label className={styles.pageSize}>
        <span>Rows</span>
        <select
          aria-label={`${itemLabel} per page`}
          onChange={(event) => onPageSizeChange(Number(event.target.value))}
          value={pageSize}
        >
          {pageSizeOptions.map((option) => (
            <option key={option} value={option}>{option}</option>
          ))}
        </select>
      </label>
      <div className={styles.controls}>
        <button
          aria-label={`Previous ${itemLabel} page`}
          disabled={busy || !hasPreviousPage}
          onClick={onPreviousPage}
          type="button"
        >
          Previous
        </button>
        <span aria-live="polite">Page {safePage}</span>
        <button
          aria-label={`Next ${itemLabel} page`}
          disabled={busy || !hasNextPage}
          onClick={onNextPage}
          type="button"
        >
          Next
        </button>
      </div>
    </nav>
  );
}
