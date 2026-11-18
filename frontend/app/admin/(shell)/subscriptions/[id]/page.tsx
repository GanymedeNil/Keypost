'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ExternalLink } from 'lucide-react';
import type { LicenseKey, SubscriptionDetail, SubscriptionInvoice } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import { useApi } from '@/lib/use-api';
import { humanize } from '@/lib/utils';
import { SubscriptionActions } from '@/components/admin/subscriptions/subscription-actions';
import { LinkStripeDialog } from '@/components/admin/subscriptions/link-stripe-dialog';
import {
  Badge,
  Button,
  Callout,
  Card,
  CardBody,
  CardHeader,
  CopyField,
  DI,
  DL,
  DataTable,
  DateText,
  EmptyState,
  ErrorState,
  JsonViewer,
  MoneyText,
  PageHeader,
  PageSkeleton,
  StatusBadge,
  TestModeBadge,
  type Column,
} from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';

function InvoicesCard({ invoices }: { invoices: SubscriptionInvoice[] }) {
  const { t, te } = useI18n();
  const columns: Column<SubscriptionInvoice>[] = [
    {
      key: 'reason',
      header: t('subscriptions.invoices.reason'),
      cell: (i) => <span className="whitespace-nowrap">{te(`subscriptions.reason.${i.billing_reason}`, humanize(i.billing_reason))}</span>,
    },
    { key: 'status', header: t('common.status'), cell: (i) => <StatusBadge status={i.status} /> },
    {
      key: 'total',
      header: t('subscriptions.invoices.total'),
      align: 'right',
      cell: (i) => <MoneyText amount={i.total} currency={i.currency} />,
    },
    {
      key: 'refunded',
      header: t('subscriptions.invoices.refunded'),
      align: 'right',
      hideBelow: 'sm',
      cell: (i) => (i.refunded_amount ? <MoneyText amount={i.refunded_amount} currency={i.currency} className="text-danger-text" /> : <span className="text-muted-foreground">—</span>),
    },
    { key: 'date', header: t('common.date'), hideBelow: 'sm', cell: (i) => <DateText value={i.created_at} style="date" className="text-muted-foreground" /> },
    {
      key: 'link',
      header: <span className="sr-only">{t('subscriptions.invoices.link')}</span>,
      align: 'right',
      cell: (i) =>
        i.invoice_url ? (
          <a
            href={i.invoice_url}
            target="_blank"
            rel="noreferrer"
            aria-label={t('subscriptions.invoices.open')}
            title={t('subscriptions.invoices.open')}
            className="inline-flex size-7 items-center justify-center rounded-md text-primary hover:bg-muted"
          >
            <ExternalLink className="size-3.5" />
          </a>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
  ];
  return (
    <Card>
      <CardHeader title={t('subscriptions.invoices.title')} />
      {invoices.length === 0 ? (
        <EmptyState title={t('subscriptions.invoices.empty')} className="py-10" />
      ) : (
        <DataTable columns={columns} rows={invoices} rowKey={(i) => i.id} />
      )}
    </Card>
  );
}

function LicenseKeysCard({ keys }: { keys: LicenseKey[] }) {
  const t = useI18n().t;
  return (
    <Card>
      <CardHeader title={t('subscriptions.keys.title')} />
      {keys.length === 0 ? (
        <EmptyState title={t('subscriptions.keys.empty')} className="py-10" />
      ) : (
        <ul className="divide-y divide-border">
          {keys.map((k) => (
            <li key={k.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5">
              <div className="min-w-0">
                <Link href={`/admin/license-keys/${k.id}`} className="font-mono text-sm font-medium text-foreground hover:text-primary">
                  {k.key_short}
                </Link>
                <p className="truncate text-xs text-muted-foreground">
                  {k.product_name} · {k.variant_name}
                </p>
              </div>
              <StatusBadge status={k.status} />
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

export default function SubscriptionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t, te } = useI18n();
  const can = useCan();
  const { data: sub, error, isLoading, mutate } = useApi<SubscriptionDetail>(`/subscriptions/${id}`);
  const [linking, setLinking] = useState(false);

  const back = { href: '/admin/subscriptions', label: t('subscriptions.detail.back') };

  if (isLoading && !sub) return <PageSkeleton />;
  if (error || !sub) return <ErrorState error={error} onRetry={() => void mutate()} />;

  return (
    <>
      <PageHeader
        title={t('subscriptions.detail.title', { id: sub.id })}
        description={`${sub.product_name} · ${sub.variant_name}`}
        back={back}
        badges={
          <>
            <StatusBadge status={sub.status} label={te(`status.${sub.status}`, sub.status_formatted)} />
            {sub.test_mode && <TestModeBadge />}
          </>
        }
        actions={<SubscriptionActions sub={sub} onChanged={() => mutate()} />}
      />

      <div className="grid gap-4 xl:grid-cols-3 xl:items-start">
        <div className="min-w-0 xl:col-span-2">
          <Card>
            <CardHeader title={t('subscriptions.summary')} />
            <CardBody className="space-y-4">
              {sub.cancelled && sub.status !== 'expired' && <Callout tone="warning">{t('subscriptions.cancelledNote')}</Callout>}
              <DL cols={2}>
                <DI label={t('subscriptions.field.customer')}>
                  <Link href={`/admin/customers/${sub.customer_id}`} className="font-medium text-primary hover:underline">
                    {sub.user_name || sub.user_email}
                  </Link>
                  <span className="block text-xs text-muted-foreground">{sub.user_email}</span>
                </DI>
                <DI label={t('subscriptions.field.order')}>
                  <Link href={`/admin/orders/${sub.order_id}`} className="text-primary hover:underline">
                    {t('subscriptions.orderLink', { id: sub.order_id })}
                  </Link>
                </DI>
                <DI label={t('subscriptions.field.product')}>
                  <Link href={`/admin/products/${sub.product_id}`} className="hover:text-primary hover:underline">
                    {sub.product_name}
                  </Link>
                </DI>
                <DI label={t('subscriptions.field.variant')}>{sub.variant_name}</DI>
                <DI label={t('subscriptions.field.quantity')}>{sub.quantity}</DI>
                <DI label={t('subscriptions.field.trialEnds')}>{sub.trial_ends_at ? <DateText value={sub.trial_ends_at} /> : null}</DI>
                <DI label={t('subscriptions.field.renews')}>{sub.renews_at ? <DateText value={sub.renews_at} /> : null}</DI>
                <DI label={t('subscriptions.field.ends')}>{sub.ends_at ? <DateText value={sub.ends_at} /> : null}</DI>
                <DI label={t('subscriptions.field.billingAnchor')}>
                  {sub.billing_anchor ? t('subscriptions.billingAnchorDay', { day: sub.billing_anchor }) : null}
                </DI>
                <DI label={t('subscriptions.field.processor')}>{sub.payment_processor ? <Badge>{sub.payment_processor}</Badge> : null}</DI>
                <DI label={t('subscriptions.field.card')}>
                  {sub.card_last_four ? (
                    <span>
                      <span className="capitalize">{sub.card_brand ?? ''}</span> ····{sub.card_last_four}
                    </span>
                  ) : null}
                </DI>
                <DI label={t('subscriptions.field.created')}>
                  <DateText value={sub.created_at} />
                </DI>
              </DL>
              {sub.pause !== null && sub.pause !== undefined && (
                <div>
                  <p className="mb-1 text-xs text-muted-foreground">{t('subscriptions.field.pause')}</p>
                  <JsonViewer value={sub.pause} maxHeight={160} />
                </div>
              )}
            </CardBody>
          </Card>
        </div>
        <div className="min-w-0 xl:col-start-3 xl:row-span-3 xl:row-start-1">
          <Card>
            <CardHeader title={t('subscriptions.stripe.title')} />
            <CardBody className="space-y-4">
              {sub.stripe_subscription_id ? (
                <div className="space-y-1.5">
                  <p className="text-xs text-muted-foreground">{t('subscriptions.stripe.subscriptionId')}</p>
                  <CopyField value={sub.stripe_subscription_id} label={t('subscriptions.stripe.subscriptionId')} />
                </div>
              ) : (
                <Callout
                  tone="warning"
                  title={t('subscriptions.stripe.notLinked')}
                  action={
                    can.write ? (
                      <Button size="sm" onClick={() => setLinking(true)}>
                        {t('subscriptions.action.link')}
                      </Button>
                    ) : undefined
                  }
                >
                  {t('subscriptions.stripe.notLinkedHint')}
                </Callout>
              )}
              {sub.stripe_customer_id && (
                <div className="space-y-1.5">
                  <p className="text-xs text-muted-foreground">{t('subscriptions.stripe.customerId')}</p>
                  <CopyField value={sub.stripe_customer_id} label={t('subscriptions.stripe.customerId')} />
                </div>
              )}
            </CardBody>
          </Card>
        </div>
        <div className="min-w-0 xl:col-span-2">
          <InvoicesCard invoices={sub.invoices ?? []} />
        </div>
        <div className="min-w-0 xl:col-span-2">
          <LicenseKeysCard keys={sub.license_keys ?? []} />
        </div>
      </div>
      {linking && (
        <LinkStripeDialog
          subscriptionId={sub.id}
          initialValue=""
          onClose={() => setLinking(false)}
          onLinked={() => {
            setLinking(false);
            void Promise.all([mutate(), invalidate('/subscriptions')]);
          }}
        />
      )}
    </>
  );
}
