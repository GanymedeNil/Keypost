'use client';

import { useApi } from '@/lib/use-api';

/** Enumeration of subscribable webhook events (`GET /webhook-events`). */
export function useWebhookEvents() {
  return useApi<string[]>('/webhook-events', undefined, { scoped: false, revalidateOnFocus: false });
}

export interface EventGroup {
  group: string;
  events: string[];
}

/** Group events by the resource prefix (`order_created` → `order`), preserving the API order. */
export function groupEvents(events: string[]): EventGroup[] {
  const map = new Map<string, string[]>();
  for (const e of events) {
    const prefix = e.split('_')[0] || 'other';
    const list = map.get(prefix);
    if (list) list.push(e);
    else map.set(prefix, [e]);
  }
  return [...map].map(([group, list]) => ({ group, events: list }));
}
