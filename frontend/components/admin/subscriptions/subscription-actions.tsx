'use client';

import { useState } from 'react';
import { Link2, Pause, Play, RefreshCw } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { invalidate } from '@/lib/swr';
import type { SubscriptionDetail } from '@/lib/types';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import { ActionMenu, Button, useConfirm } from '@/components/kit';
import { LinkStripeDialog } from './link-stripe-dialog';

export function SubscriptionActions({ sub, onChanged }: { sub: SubscriptionDetail; onChanged: () => Promise<unknown> }) {
  const t = useT();
  const can = useCan();
  const confirm = useConfirm();
  const { run, pending } = useAction();
  const [linking, setLinking] = useState(false);

  if (!can.write) return null;

  const finished = sub.status === 'cancelled' || sub.status === 'expired';
  const canCancel = !sub.cancelled && !finished;
  const canResume = sub.status === 'paused' || (sub.cancelled && sub.status !== 'expired');
  const linked = !!sub.stripe_subscription_id;

  async function refresh() {
    await Promise.all([onChanged(), invalidate('/subscriptions')]);
  }

  async function post(action: 'cancel' | 'resume' | 'sync', success: string) {
    const res = await run(() => adminApi.post(`/subscriptions/${sub.id}/${action}`), { success });
    if (res.ok) await refresh();
  }

  async function cancel() {
    const ok = await confirm({
      title: t('subscriptions.cancel.title'),
      description: t('subscriptions.cancel.body'),
      confirmLabel: t('subscriptions.cancel.confirm'),
      cancelLabel: t('subscriptions.cancel.keep'),
      tone: 'danger',
    });
    if (ok) await post('cancel', t('subscriptions.toast.cancelled'));
  }

  return (
    <>
      {!linked && (
        <Button variant="primary" icon={<Link2 className="size-4" />} onClick={() => setLinking(true)}>
          {t('subscriptions.action.link')}
        </Button>
      )}
      {linked && (
        <Button icon={<RefreshCw className="size-4" />} loading={pending} onClick={() => post('sync', t('subscriptions.toast.synced'))}>
          {t('subscriptions.action.sync')}
        </Button>
      )}
      {canResume && (
        <Button icon={<Play className="size-4" />} disabled={pending} onClick={() => post('resume', t('subscriptions.toast.resumed'))}>
          {t('subscriptions.action.resume')}
        </Button>
      )}
      {canCancel && (
        <Button variant="danger-outline" icon={<Pause className="size-4" />} disabled={pending} onClick={cancel}>
          {t('subscriptions.action.cancel')}
        </Button>
      )}
      {linked && <ActionMenu items={[{ key: 'relink', label: t('subscriptions.action.relink'), icon: <Link2 />, onSelect: () => setLinking(true) }]} />}
      {linking && (
        <LinkStripeDialog
          subscriptionId={sub.id}
          initialValue={sub.stripe_subscription_id ?? ''}
          onClose={() => setLinking(false)}
          onLinked={() => {
            setLinking(false);
            void refresh();
          }}
        />
      )}
    </>
  );
}
