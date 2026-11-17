'use client';

import { useReducer, useRef } from 'react';
import useSWR from 'swr';
import { adminApi, request, type ApiError } from '@/lib/api';
import type { ImportLog } from '@/lib/types';
import { unwrapList } from './utils';

function reducer(state: ImportLog[], entries: ImportLog[]): ImportLog[] {
  const last = state.length > 0 ? state[state.length - 1].id : 0;
  const fresh = entries.filter((e) => e.id > last);
  return fresh.length > 0 ? [...state, ...fresh] : state;
}

/**
 * Incrementally accumulates `GET /imports/{id}/logs?after=<lastId>`.
 * While `live`, it polls every 1.5s. When `live` flips to false the SWR key changes, which
 * triggers exactly one more fetch to pick up the final entries written as the job finished.
 */
export function useJobLogs(jobId: number, live: boolean) {
  const [logs, append] = useReducer(reducer, []);
  const cursor = useRef(0);

  const { error } = useSWR<ImportLog[], ApiError>(
    [adminApi.url(`/imports/${jobId}/logs`), live],
    async ([url]: [string, boolean]) => unwrapList(await request<ImportLog[] | { data: ImportLog[] }>(url + (cursor.current ? `?after=${cursor.current}` : ''))),
    {
      refreshInterval: live ? 1500 : 0,
      dedupingInterval: 0,
      revalidateOnFocus: false,
      onSuccess: (entries) => {
        if (entries.length === 0) return;
        cursor.current = Math.max(cursor.current, entries[entries.length - 1].id);
        append(entries);
      },
    },
  );

  return { logs, error };
}
