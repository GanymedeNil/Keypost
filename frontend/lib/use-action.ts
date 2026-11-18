'use client';

import { useCallback, useState } from 'react';
import { ApiError } from './api';
import { useToast } from '@/components/kit/toast';
import { useT } from './i18n/provider';
import { errorText } from './error-text';

export type ActionResult<T> = { ok: true; data: T } | { ok: false; error: unknown };

/**
 * Wrap a mutation: tracks `pending`, collects validation `fieldErrors` and shows toasts.
 *
 *   const { run, pending, fieldErrors } = useAction();
 *   const res = await run(() => adminApi.post('/x', body), { success: t('toast.saved') });
 *   if (res.ok) …
 */
export function useAction() {
  const t = useT();
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const run = useCallback(
    async <T,>(fn: () => Promise<T>, opts: { success?: string; silent?: boolean } = {}): Promise<ActionResult<T>> => {
      setPending(true);
      setFieldErrors({});
      try {
        const data = await fn();
        if (opts.success) toast.success(opts.success);
        return { ok: true, data };
      } catch (error) {
        if (error instanceof ApiError && Object.keys(error.fields).length > 0) setFieldErrors(error.fields);
        if (!opts.silent) toast.error(errorText(error, t));
        return { ok: false, error };
      } finally {
        setPending(false);
      }
    },
    [t, toast],
  );

  const clearErrors = useCallback(() => setFieldErrors({}), []);
  return { run, pending, fieldErrors, clearErrors };
}
