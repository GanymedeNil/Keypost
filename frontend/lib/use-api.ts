'use client';

import useSWR, { type SWRConfiguration } from 'swr';
import { adminApi, type ApiError, fetcher, type Query } from './api';
import { useScope } from '@/components/admin/admin-context';
import { useUrlState } from './url-state';
import type { Paginated } from './types';

export interface UseApiOptions extends SWRConfiguration {
  /** Add `test_mode` (current global switch) to the request. Default true. */
  scoped?: boolean;
  /** Also add `store_id` when a store is selected. Default false for single resources. */
  storeScoped?: boolean;
}

/** GET a single admin resource (or any non-paginated endpoint). Pass `null` to skip. */
export function useApi<T>(path: string | null, query?: Query, opts: UseApiOptions = {}) {
  const { testMode, storeId } = useScope();
  const { scoped = true, storeScoped = false, ...swr } = opts;
  const full: Query = {
    ...(scoped ? { test_mode: testMode } : {}),
    ...(storeScoped && storeId ? { store_id: storeId } : {}),
    ...query,
  };
  const key = path === null ? null : adminApi.url(path, full);
  return useSWR<T, ApiError>(key, fetcher, swr);
}

export interface UseListOptions {
  /** Filter keys read from the URL and forwarded to the API (e.g. `['status','product_id']`). */
  filters?: string[];
  defaultSort?: string;
  perPage?: number;
  /** Extra fixed query params (e.g. `{ customer_id: 3 }`). */
  query?: Query;
  /** Set false to skip fetching. */
  enabled?: boolean;
  /** Poll interval in ms. */
  refreshInterval?: number;
  /** Read `page`, `q`, `sort` and filters from the URL (default) — false keeps them in the options only. */
  urlSync?: boolean;
}

/**
 * Paginated list synced with the URL query string (`page`, `q`, `sort`, plus `filters`).
 * Passes `test_mode` and the selected `store_id` automatically.
 */
export function useList<T>(path: string, opts: UseListOptions = {}) {
  const { filters = [], defaultSort, perPage = 25, query: extra, enabled = true, refreshInterval } = opts;
  const { get, set } = useUrlState();
  const { testMode, storeId } = useScope();

  const page = Math.max(1, Number(get('page')) || 1);
  const q = get('q');
  const sort = get('sort') || defaultSort || '';
  const filterValues: Record<string, string> = {};
  for (const f of filters) filterValues[f] = get(f);

  const query: Query = {
    page,
    per_page: perPage,
    q,
    sort,
    test_mode: testMode,
    store_id: storeId,
    ...filterValues,
    ...extra,
  };
  const key = enabled ? adminApi.url(path, query) : null;
  const swr = useSWR<Paginated<T>, ApiError>(key, fetcher, { keepPreviousData: true, refreshInterval });

  const hasFilters = !!q || filters.some((f) => filterValues[f]);
  return {
    rows: swr.data?.data,
    meta: swr.data?.meta,
    isLoading: swr.isLoading,
    isValidating: swr.isValidating,
    error: swr.error,
    mutate: swr.mutate,
    page,
    q,
    sort,
    values: filterValues,
    hasFilters,
    /** The resolved query (without `page`/`per_page`), handy for building export links. */
    exportQuery: { ...query, page: undefined, per_page: undefined } as Query,
    setPage: (p: number) => set({ page: p }, { keepPage: true }),
    setSearch: (v: string) => set({ q: v }),
    setSort: (v: string) => set({ sort: v === defaultSort ? '' : v }),
    setFilter: (k: string, v: string) => set({ [k]: v }),
    clearFilters: () => set(Object.fromEntries(['q', ...filters].map((k) => [k, '']))),
    set,
  };
}

export { useAction, type ActionResult } from './use-action';
