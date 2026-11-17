'use client';

import { RotateCw } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { DeliveryDetail } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { Button, Callout, DateText, DI, DL, Drawer, ErrorState, JsonViewer, SkeletonText, StatusBadge } from '@/components/ui';
import { useCan } from '@/components/admin/admin-context';
import { CodeBlock } from './code-block';

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="mb-1.5 text-xs font-medium text-fg">{title}</h3>
      {children}
    </section>
  );
}

/** Delivery detail side sheet (payload, response, error) with a "Redeliver" action. */
export function DeliveryDrawer({ id, onClose }: { id: number | null; onClose: () => void }) {
  const { t } = useI18n();
  const can = useCan();
  const { data, error, isLoading, mutate } = useApi<DeliveryDetail>(id === null ? null : `/webhook-deliveries/${id}`, undefined, { scoped: false });
  const { run, pending } = useAction();

  async function redeliver() {
    if (id === null) return;
    const res = await run(() => adminApi.post(`/webhook-deliveries/${id}/redeliver`), { success: t('webhooks.drawer.redelivered') });
    if (res.ok) {
      await Promise.all([mutate(), invalidate('/webhook-deliveries', '/webhooks')]);
    }
  }

  return (
    <Drawer
      open={id !== null}
      onOpenChange={(o) => !o && onClose()}
      title={t('webhooks.drawer.title', { id: id ?? '' })}
      description={data?.event_name}
      width="max-w-2xl"
      footer={
        can.write && data ? (
          <Button variant="primary" icon={<RotateCw className="size-3.5" />} loading={pending} onClick={redeliver}>
            {t('webhooks.drawer.redeliver')}
          </Button>
        ) : undefined
      }
    >
      {error ? (
        <ErrorState error={error} onRetry={() => mutate()} />
      ) : isLoading || !data ? (
        <SkeletonText lines={8} />
      ) : (
        <div className="space-y-5">
          <DL cols={2}>
            <DI label={t('common.status')}>
              <StatusBadge status={data.status} />
            </DI>
            <DI label={t('webhooks.test.statusCode')}>{data.last_status_code ?? '—'}</DI>
            <DI label={t('webhooks.drawer.endpoint')} className="sm:col-span-2">
              <span className="break-all font-mono text-xs">{data.webhook_url}</span>
            </DI>
            <DI label={t('webhooks.drawer.resource')}>
              {data.resource_type ? (
                <span className="font-mono text-xs">
                  {data.resource_type}
                  {data.resource_id !== null ? ` #${data.resource_id}` : ''}
                </span>
              ) : null}
            </DI>
            <DI label={t('webhooks.test.attempts')}>{data.attempts}</DI>
            <DI label={t('webhooks.test.duration')}>{data.duration_ms === null ? '—' : t('webhooks.durationMs', { ms: data.duration_ms })}</DI>
            <DI label={t('webhooks.drawer.nextAttempt')}>
              <DateText value={data.next_attempt_at} />
            </DI>
            <DI label={t('common.created')}>
              <DateText value={data.created_at} />
            </DI>
            <DI label={t('webhooks.drawer.delivered')}>
              <DateText value={data.delivered_at} />
            </DI>
          </DL>
          {data.last_error && (
            <Callout tone="danger" title={t('webhooks.drawer.error')}>
              <span className="break-words font-mono text-xs">{data.last_error}</span>
            </Callout>
          )}
          <Section title={t('webhooks.drawer.payload')}>
            {data.payload ? <JsonViewer value={data.payload} maxHeight={360} /> : <p className="text-xs text-subtle">{t('webhooks.drawer.noPayload')}</p>}
          </Section>
          <Section title={t('webhooks.drawer.response')}>
            {data.last_response?.trim() ? <CodeBlock value={data.last_response} /> : <p className="text-xs text-subtle">{t('webhooks.test.noResponse')}</p>}
          </Section>
        </div>
      )}
    </Drawer>
  );
}
