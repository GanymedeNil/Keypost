'use client';

import { useEffect, type RefObject } from 'react';

/** Keeps a scroll container pinned to its bottom while `enabled` and the item count grows. */
export function useScrollToEnd(ref: RefObject<HTMLElement | null>, enabled: boolean, itemCount: number) {
  useEffect(() => {
    const el = ref.current;
    if (enabled && el) el.scrollTop = el.scrollHeight;
  }, [ref, enabled, itemCount]);
}
