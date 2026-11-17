'use client';

import { RotateCw } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { StripeEventDetail } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { Button, Callout, DateText, DI, DL, Drawer, ErrorState, JsonViewer, SkeletonText, StatusBadge } from '@/components/ui';
import { useCan } from '@/components/admin/admin-context';
import { LivemodeBadge } from './livemode-badge';

/** Stripe event detail side sheet with payload viewer and a "Replay" action. */
export function EventDrawer({ id, onClose }: { id: number | null; onClose: () => void }) {
  const { t } = useI18n();
  const can = useCan();
  const { data, error, isLoading, mutate } = useApi<StripeEventDetail>(id === null ? null : `/stripe-events/${id}`, undefined, { scoped: false });
  const { run, pending } = useAction();

  async function replay() {
    if (id === null) return;
    const res = await run(() => adminApi.post(`/stripe-events/${id}/replay`), { success: t('stripe.events.replayed') });
    if (res.ok) await Promise.all([mutate(), invalidate('/stripe-events', '/orders', '/subscriptions', '/license-keys')]);
  }

  const hasPayload = data?.payload !== undefined && data.payload !== null && data.payload !== '';

  return (
    <Drawer
      open={id !== null}
      onOpenChange={(o) => !o && onClose()}
      title={t('stripe.events.drawerTitle', { id: id ?? '' })}
      description={data?.type}
      width="max-w-2xl"
      footer={
        can.write && data ? (
          <Button variant="primary" icon={<RotateCw className="size-3.5" />} loading={pending} onClick={replay}>
            {t('stripe.events.replay')}
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
            <DI label={t('stripe.events.col.mode')}>
              <LivemodeBadge livemode={data.livemode} />
            </DI>
            <DI label={t('stripe.events.account')}>{data.account ? <code className="break-all text-xs">{data.account}</code> : null}</DI>
            <DI label={t('stripe.events.col.attempts')}>{data.attempts}</DI>
            <DI label={t('stripe.events.col.received')}>
              <DateText value={data.received_at} />
            </DI>
            <DI label={t('stripe.events.col.processed')}>
              <DateText value={data.processed_at} />
            </DI>
          </DL>
          {data.error && (
            <Callout tone="danger" title={t('stripe.events.error')}>
              <span className="break-words font-mono text-xs">{data.error}</span>
            </Callout>
          )}
          <section>
            <h3 className="mb-1.5 text-xs font-medium text-fg">{t('stripe.events.payload')}</h3>
            {hasPayload ? <JsonViewer value={data.payload} maxHeight={480} /> : <p className="text-xs text-subtle">{t('stripe.events.noPayload')}</p>}
          </section>
        </div>
      )}
    </Drawer>
  );
}
