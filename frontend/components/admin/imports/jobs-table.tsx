'use client';

import Link from 'next/link';
import useSWR from 'swr';
import { adminApi, fetcher, type ApiError } from '@/lib/api';
import { formatNumber } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { ImportJob, Paginated } from '@/lib/types';
import { useUrlState } from '@/lib/url-state';
import { Badge, Card, CardHeader, DataTable, DateText, Progress, StatusBadge, type Column } from '@/components/ui';
import { formatDuration, isActive, jobDuration, progressTone, useImportLabels } from './utils';

const PER_PAGE = 10;

function ProgressCell({ job }: { job: ImportJob }) {
  const { t } = useI18n();
  const labels = useImportLabels();
  const p = job.progress;
  if (!p) return <span className="text-subtle">—</span>;
  const finished = job.status === 'completed';
  const value = finished ? 1 : p.total > 0 ? p.done / p.total : 0;
  return (
    <div className="w-full space-y-1 sm:w-40">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="truncate text-muted">{labels.stage(p.stage)}</span>
        {p.total > 0 && !finished && <span className="shrink-0 tabular-nums text-subtle">{t('imports.progress.items', { done: p.done, total: p.total })}</span>}
      </div>
      <Progress value={value} max={1} tone={progressTone(job.status)} label={labels.stage(p.stage)} />
    </div>
  );
}

function CountsCell({ job }: { job: ImportJob }) {
  const { t, locale } = useI18n();
  const labels = useImportLabels();
  const entries = Object.entries(job.progress?.counts ?? {}).filter(([, n]) => n > 0);
  if (entries.length === 0) return <span className="text-subtle">—</span>;
  const shown = entries.slice(0, 3);
  return (
    <span className="text-xs text-muted">
      {shown.map(([k, n]) => `${labels.stage(k)} ${formatNumber(n, locale)}`).join(' · ')}
      {entries.length > shown.length && <span className="text-subtle"> · {t('imports.countsMore', { count: entries.length - shown.length })}</span>}
    </span>
  );
}

/** Paginated import jobs; refreshes every 3s while any job on the page is pending/running. */
export function JobsTable() {
  const { t } = useI18n();
  const labels = useImportLabels();
  const { get, set } = useUrlState();
  const page = Math.max(1, Number(get('page')) || 1);
  const key = adminApi.url('/imports', { page, per_page: PER_PAGE });
  const { data, error, isLoading, mutate } = useSWR<Paginated<ImportJob>, ApiError>(key, fetcher, {
    keepPreviousData: true,
    dedupingInterval: 1000,
    refreshInterval: (latest) => (latest?.data.some((j) => isActive(j.status)) ? 3000 : 0),
  });

  const columns: Column<ImportJob>[] = [
    {
      key: 'id',
      header: t('imports.col.job'),
      cell: (j) => (
        <Link href={`/admin/imports/${j.id}`} className="font-medium text-fg hover:text-accent-text">
          #{j.id}
        </Link>
      ),
    },
    {
      key: 'kind',
      header: t('imports.col.kind'),
      cell: (j) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <span className="whitespace-nowrap">{labels.kind(j.kind)}</span>
          {j.options?.dry_run === true && <Badge tone="info">{t('imports.dryRunBadge')}</Badge>}
        </span>
      ),
    },
    {
      key: 'status',
      header: t('common.status'),
      cell: (j) => (
        <div className="space-y-2">
          <StatusBadge status={j.status} />
          <div className="w-28 sm:hidden">
            <ProgressCell job={j} />
          </div>
        </div>
      ),
    },
    { key: 'progress', header: t('imports.col.progress'), hideBelow: 'sm', cell: (j) => <ProgressCell job={j} /> },
    { key: 'counts', header: t('imports.col.counts'), hideBelow: 'xl', cell: (j) => <CountsCell job={j} /> },
    { key: 'by', header: t('imports.col.createdBy'), hideBelow: 'lg', cell: (j) => <span className="text-muted">{j.created_by ?? '—'}</span> },
    { key: 'started', header: t('imports.col.started'), hideBelow: 'md', cell: (j) => <DateText value={j.started_at ?? j.created_at} relative className="text-muted" /> },
    {
      key: 'finished',
      header: t('imports.col.finished'),
      hideBelow: 'lg',
      cell: (j) => {
        const ms = jobDuration(j);
        return j.finished_at ? (
          <span className="flex flex-col text-muted">
            <DateText value={j.finished_at} relative />
            {ms !== null && <span className="text-xs text-subtle">{formatDuration(ms)}</span>}
          </span>
        ) : (
          <span className="text-subtle">—</span>
        );
      },
    },
  ];

  return (
    <Card>
      <CardHeader title={t('imports.jobs.title')} description={t('imports.jobs.desc')} />
      <DataTable
        columns={columns}
        rows={data?.data}
        rowKey={(j) => j.id}
        loading={isLoading}
        error={error}
        onRetry={() => void mutate()}
        meta={data?.meta}
        onPageChange={(p) => set({ page: p }, { keepPage: true })}
        rowHref={(j) => `/admin/imports/${j.id}`}
        empty={{ title: t('imports.jobs.empty'), description: t('imports.jobs.emptyHint') }}
        skeletonRows={4}
      />
    </Card>
  );
}
