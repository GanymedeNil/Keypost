'use client';

import { useCallback, useSyncExternalStore } from 'react';

/**
 * Tiny localStorage-backed store usable with useSyncExternalStore (hydration safe:
 * the server snapshot is always the fallback, the client re-renders with the stored value).
 */
const listeners = new Map<string, Set<() => void>>();

function emit(key: string) {
  listeners.get(key)?.forEach((l) => l());
}

export function readStored(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writeStored(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    /* storage unavailable (private mode…) */
  }
  emit(key);
}

function subscribe(key: string, cb: () => void) {
  let set = listeners.get(key);
  if (!set) {
    set = new Set();
    listeners.set(key, set);
  }
  set.add(cb);
  const onStorage = (e: StorageEvent) => {
    if (e.key === key) cb();
  };
  window.addEventListener('storage', onStorage);
  return () => {
    set.delete(cb);
    window.removeEventListener('storage', onStorage);
  };
}

/** Persisted string value. Returns `[value, setValue]`; `value` is `fallback` on the server and before hydration. */
export function usePersistedString(key: string, fallback: string | null = null): [string | null, (v: string | null) => void] {
  const value = useSyncExternalStore(
    useCallback((cb) => subscribe(key, cb), [key]),
    () => readStored(key) ?? fallback,
    () => fallback,
  );
  const set = useCallback((v: string | null) => writeStored(key, v), [key]);
  return [value, set];
}
