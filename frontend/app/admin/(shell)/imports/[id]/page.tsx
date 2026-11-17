'use client';

import { useParams } from 'next/navigation';
import { XCircle } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import type { ImportJob } from '@/lib/types';
import { invalidate } from '@/lib/swr';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { useCan } from '@/components/admin/admin-context';
import { JobDetails } from '@/components/admin/imports/job-details';
import { JobLogs } from '@/components/admin/imports/job-logs';
import { JobProgress } from '@/components/admin/imports/job-progress';
import { isActive } from '@/components/admin/imports/utils';
import { Button, Callout, ErrorState, PageHeader, PageSkeleton, StatusBadge, useConfirm } from '@/components/ui';

export default function ImportJobPage() {
  const { t } = useI18n();
  const { id } = useParams<{ id: string }>();
  const can = useCan();
  const confirm = useConfirm();
  const { run, pending } = useAction();
  const { data: job, error, mutate } = useApi<ImportJob>(`/imports/${id}`, undefined, {
    scoped: false,
    dedupingInterval: 500,
    refreshInterval: (latest) => (isActive(latest?.status) ? 1500 : 0),
  });

  if (error && !job) {
    return (
      <>
        <PageHeader title={t('imports.job.title', { id })} back={{ href: '/admin/imports', label: t('imports.job.back') }} />
        <ErrorState error={error} onRetry={() => void mutate()} />
      </>
    );
  }
  if (!job) return <PageSkeleton />;

  const active = isActive(job.status);

  async function cancel() {
    const ok = await confirm({
      title: t('imports.job.cancelTitle', { id }),
      description: t('imports.job.cancelBody'),
      confirmLabel: t('imports.job.cancelConfirm'),
      cancelLabel: t('imports.job.keepRunning'),
    });
    if (!ok) return;
    const res = await run(() => adminApi.post<ImportJob>(`/imports/${id}/cancel`), { success: t('imports.toast.cancelled') });
    if (res.ok) {
      await mutate();
      void invalidate('/imports');
    }
  }

  return (
    <>
      <PageHeader
        title={t('imports.job.title', { id })}
        back={{ href: '/admin/imports', label: t('imports.job.back') }}
        badges={<StatusBadge status={job.status} />}
        actions={
          active &&
          can.write && (
            <Button variant="danger-outline" icon={<XCircle className="size-4" />} loading={pending} onClick={() => void cancel()}>
              {t('imports.job.cancel')}
            </Button>
          )
        }
      />
      <div className="space-y-4">
        {job.status === 'failed' && (
          <Callout tone="danger" title={t('imports.job.failedTitle')}>
            {job.error ?? t('errors.generic')}
          </Callout>
        )}
        <div className="grid gap-4 xl:grid-cols-3">
          <div className="min-w-0 xl:col-span-2">
            <JobProgress job={job} />
          </div>
          <JobDetails job={job} />
        </div>
        <JobLogs jobId={job.id} live={active} />
      </div>
    </>
  );
}
