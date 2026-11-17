'use client';

import { useCallback, useState } from 'react';

/**
 * Server-side validation errors that disappear for a field as soon as the user edits it.
 * `touch(key)` on change, `reset()` right before (re)submitting.
 */
export function useFieldErrors(server: Record<string, string>, local: Record<string, string> = {}) {
  const [touched, setTouched] = useState<string[]>([]);
  const touch = useCallback((key: string) => setTouched((cur) => (cur.includes(key) ? cur : [...cur, key])), []);
  const reset = useCallback(() => setTouched([]), []);
  const errors: Record<string, string> = { ...local };
  for (const [k, v] of Object.entries(server)) if (!touched.includes(k)) errors[k] = v;
  return { errors, touch, reset };
}
