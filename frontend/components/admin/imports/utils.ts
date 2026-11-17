'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { humanize } from '@/lib/utils';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { ImportJob, ImportStatus } from '@/lib/types';

/** Lemon Squeezy import stages, in the order the backend processes them. */
export const LS_STAGES = ['stores', 'products', 'variants', 'prices', 'files', 'customers', 'orders', 'subscriptions', 'license_keys'] as const;
export type LsResource = (typeof LS_STAGES)[number];

/** Resources that should already exist (selected or imported earlier) for a resource to import cleanly. */
export const RESOURCE_DEPS: Record<LsResource, LsResource[]> = {
  stores: [],
  products: ['stores'],
  variants: ['products'],
  prices: ['variants'],
  files: ['variants'],
  customers: ['stores'],
  orders: ['customers', 'variants', 'prices'],
  subscriptions: ['customers', 'orders', 'variants', 'prices'],
  license_keys: ['orders', 'variants'],
};

export function isActive(status: ImportStatus | undefined): boolean {
  return status === 'pending' || status === 'running';
}

export function isLemonSqueezyKind(kind: string): boolean {
  return kind.startsWith('lemonsqueezy');
}

/** `75000` -> `1m 15s`. */
export function formatDuration(ms: number): string {
  const total = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h) return `${h}h ${m}m`;
  if (m) return `${m}m ${s}s`;
  return `${s}s`;
}

export function jobDuration(job: Pick<ImportJob, 'started_at' | 'finished_at'>): number | null {
  if (!job.started_at || !job.finished_at) return null;
  const ms = new Date(job.finished_at).getTime() - new Date(job.started_at).getTime();
  return Number.isNaN(ms) ? null : ms;
}

export function progressTone(status: ImportStatus): 'accent' | 'success' | 'danger' | 'warning' {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'danger';
  if (status === 'cancelled') return 'warning';
  return 'accent';
}

/** Localised labels for job kinds / stages / option keys with a humanised fallback. */
export function useImportLabels() {
  const { te } = useI18n();
  return {
    kind: (kind: string) => te(`imports.kind.${kind}`, humanize(kind)),
    stage: (stage: string) => te(`imports.stage.${stage}`, humanize(stage)),
    option: (key: string) => te(`imports.opt.${key}`, humanize(key)),
  };
}

/** After a job was created: refresh lists and open the live job page (if the response carries an id). */
export function useOpenJob() {
  const router = useRouter();
  return useCallback(
    (job: Partial<ImportJob> | undefined) => {
      void invalidate('/imports');
      if (job?.id) router.push(`/admin/imports/${job.id}`);
    },
    [router],
  );
}

/** Log lines may come back as a bare array or wrapped as `{ data: [] }`. */
export function unwrapList<T>(payload: T[] | { data: T[] } | null | undefined): T[] {
  if (Array.isArray(payload)) return payload;
  if (payload && Array.isArray(payload.data)) return payload.data;
  return [];
}
