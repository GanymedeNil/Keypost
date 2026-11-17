'use client';

import { useCallback } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

export type ParamPatch = Record<string, string | number | boolean | null | undefined>;

/** Read/write query params of the current URL (filters, page, tab…) without scrolling. */
export function useUrlState() {
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();

  const get = useCallback((key: string) => params.get(key) ?? '', [params]);

  /** Patch params. Empty/null/undefined values are removed. Resets `page` unless it is part of the patch. */
  const set = useCallback(
    (patch: ParamPatch, opts: { keepPage?: boolean } = {}) => {
      const next = new URLSearchParams(params.toString());
      for (const [k, v] of Object.entries(patch)) {
        if (v === null || v === undefined || v === '' || v === false) next.delete(k);
        else next.set(k, String(v));
      }
      if (!opts.keepPage && !('page' in patch)) next.delete('page');
      if (next.get('page') === '1') next.delete('page');
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [params, pathname, router],
  );

  return { params, get, set, pathname };
}
