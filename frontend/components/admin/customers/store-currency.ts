'use client';

import { useCallback } from 'react';
import { useAdmin } from '../admin-context';

/** Customer aggregates (revenue, MRR) carry no currency in the contract: use the owning store's currency. */
export function useStoreCurrency() {
  const { stores } = useAdmin();
  return useCallback((storeId: number) => stores.find((s) => s.id === storeId)?.currency ?? 'USD', [stores]);
}
