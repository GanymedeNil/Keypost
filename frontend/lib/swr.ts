import { mutate as globalMutate } from 'swr';
import { ADMIN_BASE } from './api';

/** Revalidate every cached admin request whose path starts with one of `prefixes` (e.g. `'/orders'`). */
export function invalidate(...prefixes: string[]) {
  // No data argument: keeps the cached data on screen and just revalidates it.
  return globalMutate((key) => typeof key === 'string' && prefixes.some((p) => key.startsWith(`${ADMIN_BASE}${p}`)));
}
