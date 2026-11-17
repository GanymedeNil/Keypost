'use client';

import { createContext, useCallback, useContext, useMemo, type ReactNode } from 'react';
import useSWR from 'swr';
import { adminApi, fetcher } from '@/lib/api';
import { usePersistedString } from '@/lib/persisted';
import type { Paginated, Role, Store, User } from '@/lib/types';

const TEST_KEY = 'df-test-mode';
const STORE_KEY = 'df-store';

export interface AdminCtx {
  user: User;
  role: Role;
  /** Role-derived permissions: write = owner|admin, owner = owner. Viewers get read-only UI. */
  can: { write: boolean; admin: boolean; owner: boolean };
  stores: Store[];
  /** Selected store (null = all stores). */
  storeId: number | null;
  setStoreId: (id: number | null) => void;
  /** Global test-mode switch; every list/detail request passes `test_mode`. */
  testMode: boolean;
  setTestMode: (on: boolean) => void;
  refreshUser: () => Promise<unknown>;
  refreshStores: () => Promise<unknown>;
}

const AdminContext = createContext<AdminCtx | null>(null);

export function AdminProvider({ user, children }: { user: User; children: ReactNode }) {
  const [testRaw, setTestRaw] = usePersistedString(TEST_KEY, '0');
  const [storeRaw, setStoreRaw] = usePersistedString(STORE_KEY, null);
  const { mutate: refreshUser } = useSWR<User>(adminApi.url('/me'), fetcher);
  const { data: storesData, mutate: refreshStores } = useSWR<Paginated<Store>>(
    adminApi.url('/stores', { per_page: 100 }),
    fetcher,
    { revalidateOnFocus: false },
  );

  const stores = useMemo(() => storesData?.data ?? [], [storesData]);
  const stored = storeRaw ? Number(storeRaw) : null;
  const storeId = stored !== null && stores.some((s) => s.id === stored) ? stored : null;

  const setStoreId = useCallback((id: number | null) => setStoreRaw(id === null ? null : String(id)), [setStoreRaw]);
  const setTestMode = useCallback((on: boolean) => setTestRaw(on ? '1' : '0'), [setTestRaw]);

  const value = useMemo<AdminCtx>(
    () => ({
      user,
      role: user.role,
      can: { write: user.role !== 'viewer', admin: user.role !== 'viewer', owner: user.role === 'owner' },
      stores,
      storeId,
      setStoreId,
      testMode: testRaw === '1',
      setTestMode,
      refreshUser: () => refreshUser(),
      refreshStores: () => refreshStores(),
    }),
    [user, stores, storeId, setStoreId, testRaw, setTestMode, refreshUser, refreshStores],
  );

  return <AdminContext.Provider value={value}>{children}</AdminContext.Provider>;
}

export function useAdmin(): AdminCtx {
  const ctx = useContext(AdminContext);
  if (!ctx) throw new Error('useAdmin must be used inside the admin shell');
  return ctx;
}

/** `const can = useCan(); {can.write && <Button/>}` */
export function useCan() {
  return useAdmin().can;
}

/** Current `{testMode, storeId}` scope used by list/detail requests. */
export function useScope() {
  const { testMode, storeId } = useAdmin();
  return { testMode, storeId };
}
