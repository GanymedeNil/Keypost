'use client';

import { Select } from '@/components/kit';
import { useAdmin } from './admin-context';

/** Store picker for create forms. Defaults are up to the caller (`storeId ?? stores[0]?.id`). */
export function StoreSelect({ value, onChange, disabled }: { value: number | null | undefined; onChange: (id: number) => void; disabled?: boolean }) {
  const { stores } = useAdmin();
  return (
    <Select value={value ?? ''} onChange={(e) => onChange(Number(e.target.value))} disabled={disabled}>
      {stores.map((s) => (
        <option key={s.id} value={s.id}>
          {s.name}
        </option>
      ))}
    </Select>
  );
}

/** Convenience: the store a new record should default to. */
export function useDefaultStoreId(): number | undefined {
  const { stores, storeId } = useAdmin();
  return storeId ?? stores[0]?.id;
}
