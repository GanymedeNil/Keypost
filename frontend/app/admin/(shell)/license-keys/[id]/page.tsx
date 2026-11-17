'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { Eye, EyeOff, KeyRound, RefreshCcw, Unplug } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { invalidate } from '@/lib/swr';
import type { LicenseKeyDetail, LicenseKeyInstance } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { useCan } from '@/components/admin/admin-context';
import { LicenseSettingsCard } from '@/components/admin/licenses/license-settings-card';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CopyButton,
  DI,
  DL,
  DataTable,
  DateText,
  EmptyState,
  ErrorState,
  Mono,
  PageHeader,
  PageSkeleton,
  StatusBadge,
  TestModeBadge,
  useConfirm,
  type Column,
} from '@/components/ui';

export default function LicenseKeyDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t, te } = useI18n();
  const can = useCan();
  const confirm = useConfirm();
  const { run, pending } = useAction();
  const { data: lk, error, isLoading, mutate } = useApi<LicenseKeyDetail>(`/license-keys/${id}`);
  const [revealed, setRevealed] = useState(false);

  if (isLoading && !lk) return <PageSkeleton />;
  if (error || !lk) return <ErrorState error={error} onRetry={() => void mutate()} />;

  const refresh = () => Promise.all([mutate(), invalidate('/license-keys')]);

  async function regenerate() {
    const ok = await confirm({
      title: t('licenses.regenerate.title'),
      description: t('licenses.regenerate.body'),
      confirmLabel: t('licenses.regenerate.confirm'),
      tone: 'danger',
    });
    if (!ok) return;
    const res = await run(() => adminApi.post(`/license-keys/${lk!.id}/regenerate`), { success: t('licenses.regenerate.done') });
    if (res.ok) {
      setRevealed(true);
      await refresh();
    }
  }

  async function deactivate(instance: LicenseKeyInstance) {
    const label = instance.name || instance.identifier;
    const ok = await confirm({
      title: t('licenses.instances.deactivateTitle', { name: label }),
      description: t('licenses.instances.deactivateBody'),
      confirmLabel: t('licenses.instances.deactivate'),
      tone: 'danger',
    });
    if (!ok) return;
    const res = await run(() => adminApi.delete(`/license-key-instances/${instance.id}`), { success: t('licenses.instances.deactivated') });
    if (res.ok) await refresh();
  }

  const instanceColumns: Column<LicenseKeyInstance>[] = [
    { key: 'name', header: t('licenses.instances.name'), cell: (i) => <span className="font-medium">{i.name || '—'}</span> },
    { key: 'identifier', header: t('licenses.instances.identifier'), hideBelow: 'sm', cell: (i) => (
        <span title={i.identifier}>
          <Mono className="inline-block max-w-40 truncate align-middle">{i.identifier}</Mono>
        </span>
      ),
    },
    { key: 'created', header: t('licenses.instances.activated'), cell: (i) => <DateText value={i.created_at} className="text-muted" /> },
    ...(can.write
      ? [
          {
            key: 'actions',
            header: <span className="sr-only">{t('common.actions')}</span>,
            align: 'right' as const,
            cell: (i: LicenseKeyInstance) => (
              <Button size="sm" variant="danger-outline" icon={<Unplug className="size-3.5" />} disabled={pending} onClick={() => deactivate(i)}>
                {t('licenses.instances.deactivate')}
              </Button>
            ),
          },
        ]
      : []),
  ];

  return (
    <>
      <PageHeader
        title={lk.key_short}
        description={`${lk.product_name} · ${lk.variant_name}`}
        back={{ href: '/admin/license-keys', label: t('licenses.detail.back') }}
        badges={
          <>
            <StatusBadge status={lk.status} label={te(`status.${lk.status}`, lk.status_formatted)} />
            {lk.test_mode && <TestModeBadge />}
          </>
        }
        actions={
          can.write && (
            <Button variant="danger-outline" icon={<RefreshCcw className="size-4" />} disabled={pending} onClick={regenerate}>
              {t('licenses.regenerate.button')}
            </Button>
          )
        }
      />

      <div className="grid gap-4 xl:grid-cols-3 xl:items-start">
        <div className="min-w-0 xl:col-span-2">
          <Card>
            <CardHeader title={t('licenses.key')} />
            <CardBody className="flex items-center gap-2">
              <KeyRound aria-hidden className="size-4 shrink-0 text-subtle" />
              <code className="min-w-0 flex-1 select-all break-all rounded-md bg-surface-2 px-2.5 py-1.5 font-mono text-sm text-fg">
                {revealed ? lk.key : lk.key_short}
              </code>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={revealed ? t('licenses.hide') : t('licenses.reveal')}
                title={revealed ? t('licenses.hide') : t('licenses.reveal')}
                aria-pressed={revealed}
                onClick={() => setRevealed((v) => !v)}
              >
                {revealed ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </Button>
              <CopyButton value={lk.key} label={t('licenses.copyKey')} />
            </CardBody>
          </Card>
        </div>
        <div className="min-w-0 xl:col-start-3 xl:row-span-3 xl:row-start-1">
          <LicenseSettingsCard key={`${lk.id}-${lk.updated_at}`} lk={lk} onSaved={() => mutate()} />
        </div>
        <div className="min-w-0 xl:col-span-2">
          <Card>
            <CardHeader title={t('licenses.instances.title')} />
            {(lk.instances ?? []).length === 0 ? (
              <EmptyState title={t('licenses.instances.empty')} description={t('licenses.instances.emptyDescription')} className="py-10" />
            ) : (
              <DataTable columns={instanceColumns} rows={lk.instances} rowKey={(i) => i.id} />
            )}
          </Card>
        </div>
        <div className="min-w-0 xl:col-span-2">
          <Card>
            <CardHeader title={t('licenses.details')} />
            <CardBody>
              <DL cols={2}>
                <DI label={t('licenses.field.customer')}>
                  {lk.customer_id ? (
                    <Link href={`/admin/customers/${lk.customer_id}`} className="font-medium text-accent-text hover:underline">
                      {lk.user_name || lk.user_email}
                    </Link>
                  ) : (
                    lk.user_name || null
                  )}
                  <span className="block text-xs text-subtle">{lk.user_email}</span>
                </DI>
                <DI label={t('licenses.field.order')}>
                  {lk.order_id ? (
                    <Link href={`/admin/orders/${lk.order_id}`} className="text-accent-text hover:underline">
                      {t('licenses.orderLink', { id: lk.order_id })}
                    </Link>
                  ) : null}
                </DI>
                <DI label={t('licenses.field.subscription')}>
                  {lk.subscription_id ? (
                    <Link href={`/admin/subscriptions/${lk.subscription_id}`} className="text-accent-text hover:underline">
                      {t('licenses.subscriptionLink', { id: lk.subscription_id })}
                    </Link>
                  ) : null}
                </DI>
                <DI label={t('licenses.field.product')}>
                  <Link href={`/admin/products/${lk.product_id}`} className="hover:text-accent-text hover:underline">
                    {lk.product_name}
                  </Link>
                </DI>
                <DI label={t('licenses.field.variant')}>{lk.variant_name}</DI>
                <DI label={t('licenses.field.created')}>
                  <DateText value={lk.created_at} />
                </DI>
                <DI label={t('licenses.field.updated')}>
                  <DateText value={lk.updated_at} />
                </DI>
              </DL>
            </CardBody>
          </Card>
        </div>
      </div>
    </>
  );
}
