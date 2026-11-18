'use client';

import { useRouter } from 'next/navigation';
import type { ComponentProps, KeyboardEvent, MouseEvent, ReactNode } from 'react';
import { ArrowDown, ArrowUp, ChevronsUpDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import type { PageMeta } from '@/lib/types';
import { Pagination } from './pagination';
import { EmptyState, ErrorState } from './states';
import { Skeleton } from './skeleton';
import { Table as UiTable, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';

/* ----------------------------------------------------------- primitives (shadcn Table) */

export function TableWrap({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('relative w-full overflow-x-auto', className)} {...props} />;
}
export const Table = UiTable;
export const THead = TableHeader;
export const TBody = TableBody;
export const TR = TableRow;
export function TH({ className, ...props }: ComponentProps<'th'>) {
  return <TableHead scope="col" className={cn('h-10 px-4 text-xs text-muted-foreground', className)} {...props} />;
}
export function TD({ className, ...props }: ComponentProps<'td'>) {
  return <TableCell className={cn('px-4 py-2.5', className)} {...props} />;
}

/* ------------------------------------------------------------ DataTable */

const hideBelow = {
  sm: 'hidden sm:table-cell',
  md: 'hidden md:table-cell',
  lg: 'hidden lg:table-cell',
  xl: 'hidden xl:table-cell',
} as const;

export interface Column<T> {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  /** Hide this column below a breakpoint (responsive tables). */
  hideBelow?: keyof typeof hideBelow;
  align?: 'left' | 'right' | 'center';
  className?: string;
  /** API sort field. Clicking the header toggles `field` / `-field`. */
  sortKey?: string;
}

export interface DataTableProps<T> {
  columns: Column<T>[];
  rows: T[] | undefined;
  rowKey: (row: T) => string | number;
  loading?: boolean;
  error?: unknown;
  onRetry?: () => void;
  meta?: PageMeta;
  onPageChange?: (page: number) => void;
  sort?: string;
  onSortChange?: (sort: string) => void;
  /** Whole-row navigation (the first cell should still contain a real link for accessibility). */
  rowHref?: (row: T) => string;
  onRowClick?: (row: T) => void;
  empty?: { title?: ReactNode; description?: ReactNode; action?: ReactNode; icon?: ReactNode };
  /** True when filters/search are active, to show the "no results" copy instead of the "nothing yet" one. */
  filtered?: boolean;
  skeletonRows?: number;
  className?: string;
}

const alignCls = { left: 'text-left', right: 'text-right', center: 'text-center' } as const;

const INTERACTIVE = 'a,button,input,select,textarea,label,[role="button"],[role="menuitem"],[data-no-row-click]';

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  loading,
  error,
  onRetry,
  meta,
  onPageChange,
  sort,
  onSortChange,
  rowHref,
  onRowClick,
  empty,
  filtered,
  skeletonRows = 8,
  className,
}: DataTableProps<T>) {
  const t = useT();
  const router = useRouter();
  const showSkeleton = loading && !rows;

  if (error && !rows) return <ErrorState error={error} onRetry={onRetry} />;

  if (!showSkeleton && rows && rows.length === 0) {
    return filtered ? (
      <EmptyState title={t('common.noResults')} description={t('common.noResultsHint')} />
    ) : (
      <EmptyState title={empty?.title ?? t('common.nothingYet')} description={empty?.description} action={empty?.action} icon={empty?.icon} />
    );
  }

  const clickable = !!(rowHref || onRowClick);
  const go = (row: T) => (rowHref ? router.push(rowHref(row)) : onRowClick?.(row));

  const handleClick = (e: MouseEvent<HTMLTableRowElement>, row: T) => {
    if ((e.target as HTMLElement).closest(INTERACTIVE) || e.metaKey || e.ctrlKey) return;
    go(row);
  };
  const handleKey = (e: KeyboardEvent<HTMLTableRowElement>, row: T) => {
    if (e.key === 'Enter' && e.target === e.currentTarget) go(row);
  };

  function sortIcon(col: Column<T>) {
    if (!col.sortKey) return null;
    if (sort === col.sortKey) return <ArrowUp className="size-3" />;
    if (sort === `-${col.sortKey}`) return <ArrowDown className="size-3" />;
    return <ChevronsUpDown className="size-3 opacity-40" />;
  }

  function toggleSort(col: Column<T>) {
    if (!col.sortKey || !onSortChange) return;
    onSortChange(sort === `-${col.sortKey}` ? col.sortKey : `-${col.sortKey}`);
  }

  return (
    <div className={cn(loading && rows && 'opacity-70 transition-opacity', className)}>
      <TableWrap>
        <Table>
          <THead>
            <TR>
              {columns.map((col) => (
                <TH
                  key={col.key}
                  aria-sort={col.sortKey ? (sort === col.sortKey ? 'ascending' : sort === `-${col.sortKey}` ? 'descending' : 'none') : undefined}
                  className={cn(col.align && alignCls[col.align], col.hideBelow && hideBelow[col.hideBelow])}
                >
                  {col.sortKey && onSortChange ? (
                    <button
                      type="button"
                      onClick={() => toggleSort(col)}
                      className={cn('inline-flex items-center gap-1 hover:text-foreground', col.align === 'right' && 'flex-row-reverse')}
                    >
                      {col.header}
                      {sortIcon(col)}
                    </button>
                  ) : (
                    col.header
                  )}
                </TH>
              ))}
            </TR>
          </THead>
          <TBody>
            {showSkeleton
              ? Array.from({ length: skeletonRows }, (_, i) => (
                  <TR key={i}>
                    {columns.map((col, ci) => (
                      <TD key={col.key} className={cn(col.hideBelow && hideBelow[col.hideBelow])}>
                        <Skeleton className={cn('h-4', ci === 0 ? 'w-28' : 'w-20', col.align === 'right' && 'ml-auto')} />
                      </TD>
                    ))}
                  </TR>
                ))
              : rows?.map((row) => (
                  <TR
                    key={rowKey(row)}
                    onClick={clickable ? (e) => handleClick(e, row) : undefined}
                    onKeyDown={clickable ? (e) => handleKey(e, row) : undefined}
                    tabIndex={clickable ? 0 : undefined}
                    className={cn(clickable && 'cursor-pointer hover:bg-muted/70 focus-visible:bg-muted')}
                  >
                    {columns.map((col) => (
                      <TD key={col.key} className={cn(col.align && alignCls[col.align], col.hideBelow && hideBelow[col.hideBelow], col.className)}>
                        {col.cell(row)}
                      </TD>
                    ))}
                  </TR>
                ))}
          </TBody>
        </Table>
      </TableWrap>
      {meta && onPageChange && !showSkeleton && meta.total > 0 && <Pagination meta={meta} onPageChange={onPageChange} />}
    </div>
  );
}
