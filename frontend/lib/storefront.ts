'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import useSWR from 'swr';
import { ApiError, fetcher, storefrontApi } from './api';
import type { StorefrontMe } from './types';

/** SWR key of the signed-in customer's orders (also used to prime the cache after verifying a magic link). */
export const ME_KEY = storefrontApi.url('/me/orders');

/** 401 (no/expired session) and 403 (someone else's order / bad signature) both mean "sign in". */
export function isAuthError(e: unknown): boolean {
  return e instanceof ApiError && (e.status === 401 || e.status === 403);
}

/** Customer session + orders/subscriptions. A 401 error means "not signed in". */
export function useMe() {
  return useSWR<StorefrontMe, ApiError>(ME_KEY, fetcher, { shouldRetryOnError: false, revalidateOnFocus: false });
}

/** Countdown helper for "resend in Ns": `start()` begins a new cooldown, `remaining` is whole seconds left. */
export function useCooldown(seconds: number) {
  const [until, setUntil] = useState(0);
  const [now, setNow] = useState(0);
  const timer = useRef<ReturnType<typeof setInterval>>(undefined);

  useEffect(() => () => clearInterval(timer.current), []);

  const start = useCallback(() => {
    const end = Date.now() + seconds * 1000;
    clearInterval(timer.current);
    setUntil(end);
    setNow(Date.now());
    timer.current = setInterval(() => {
      const n = Date.now();
      setNow(n);
      if (n >= end) clearInterval(timer.current);
    }, 1000);
  }, [seconds]);

  return { remaining: Math.max(0, Math.ceil((until - now) / 1000)), start };
}
