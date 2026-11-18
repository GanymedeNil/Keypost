'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { CreditCard, ExternalLink, Pencil } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { formatDate } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { CustomerDetail, LicenseKey, Order, Subscription } from '@/lib/types';
import { useUrlState } from '@/lib/url-state';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { useCan } from '@/components/admin/admin-context';
import { EditCustomerDialog } from '@/components/admin/customers/customer-dialog';
import { useStoreCurrency } from '@/components/admin/customers/store-currency';
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
  ErrorState,
  MoneyText,
  PageHeader,
  PageSkeleton,
  StatusBadge,
  Tabs,
  TestModeBadge,
  type Column,
} from '@/components/kit';

type TabKey = 'orders' | 'subscriptions' | 'keys';
const TABS: TabKey[] = ['orders', 'subscriptions', 'keys'];

function Stat({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-0.5 truncate text-lg font-semibold tabular-nums">{children}</p>
    </div>
  );
}

export default function CustomerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t, locale } = useI18n();
  const can = useCan();
  const storeCurrency = useStoreCurrency();
  const { get, set } = useUrlState();
  const { run, pending: portalPending } = useAction();
  const { data: customer, error, isLoading, mutate } = useApi<CustomerDetail>(`/customers/${id}`);
  const [editing, setEditing] = useState(false);

  const tabParam = get('tab');
  const tab: TabKey = TABS.includes(tabParam as TabKey) ? (tabParam as TabKey) : 'orders';

  if (error && !customer) {
    return (
      <>
        <PageHeader title={t('customers.title')} back={{ href: '/admin/customers', label: t('customers.detail.back') }} />
        <Card>
          <ErrorState error={error} onRetry={() => void mutate()} />
        </Card>
      </>
    );
  }
  if (isLoading || !customer) return <PageSkeleton />;

  const currency = storeCurrency(customer.store_id);
  const location = [customer.city, customer.region, customer.country].filter(Boolean).join(', ');
  const stripeUrl = customer.stripe_customer_id
    ? `https://dashboard.stripe.com${customer.test_mode ? '/test' : ''}/customers/${customer.stripe_customer_id}`
    : null;

  async function openPortal() {
    // Open the tab synchronously (inside the click) so popup blockers allow it, then point it at the portal.
    const win = window.open('about:blank', '_blank');
    const res = await run(() => adminApi.post<{ url: string }>(`/customers/${id}/portal-link`));
    if (!res.ok) {
      win?.close();
      return;
    }
    if (win) {
      win.opener = null;
      win.location.href = res.data.url;
    } else {
      window.open(res.data.url, '_blank', 'noopener');
    }
  }

  const orderColumns: Column<Order>[] = [
    {
      key: 'order',
      header: t('customers.col.order'),
      cell: (o) => (
        <Link href={`/admin/orders/${o.id}`} className="font-medium hover:text-primary">
          #{o.order_number}
        </Link>
      ),
    },
    {
      key: 'item',
      header: t('customers.col.item'),
      hideBelow: 'sm',
      cell: (o) => (o.first_item ? `${o.first_item.product_name} — ${o.first_item.variant_name}` : '—'),
    },
    { key: 'total', header: t('common.total'), align: 'right', cell: (o) => <MoneyText amount={o.total} currency={o.currency} className="font-medium" /> },
    { key: 'status', header: t('common.status'), cell: (o) => <StatusBadge status={o.status} /> },
    { key: 'created', header: t('common.created'), hideBelow: 'md', cell: (o) => <DateText value={o.created_at} className="text-muted-foreground" /> },
  ];

  const subColumns: Column<Subscription>[] = [
    {
      key: 'product',
      header: t('customers.col.product'),
      cell: (s) => (
        <Link href={`/admin/subscriptions/${s.id}`} className="font-medium hover:text-primary">
          {s.product_name} — {s.variant_name}
        </Link>
      ),
    },
    { key: 'status', header: t('common.status'), cell: (s) => <StatusBadge status={s.status} /> },
    { key: 'renews', header: t('customers.col.renews'), hideBelow: 'sm', cell: (s) => <DateText value={s.renews_at} style="date" className="text-muted-foreground" /> },
    { key: 'ends', header: t('customers.col.ends'), hideBelow: 'md', cell: (s) => <DateText value={s.ends_at} style="date" className="text-muted-foreground" /> },
  ];

  const keyColumns: Column<LicenseKey>[] = [
    {
      key: 'key',
      header: t('customers.col.key'),
      cell: (k) => (
        <div className="flex items-center gap-1">
          <Link href={`/admin/license-keys/${k.id}`} className="font-mono text-xs font-medium hover:text-primary" title={k.key}>
            {k.key_short || k.key}
          </Link>
          <CopyButton value={k.key} label={t('customers.col.key')} />
        </div>
      ),
    },
    { key: 'product', header: t('customers.col.product'), hideBelow: 'md', cell: (k) => `${k.product_name} — ${k.variant_name}` },
    { key: 'status', header: t('common.status'), cell: (k) => <StatusBadge status={k.status} /> },
    {
      key: 'usage',
      header: t('customers.col.usage'),
      align: 'right',
      hideBelow: 'sm',
      cell: (k) => <span className="tabular-nums">{k.instances_count} / {k.activation_limit ?? '∞'}</span>,
    },
    { key: 'expires', header: t('customers.col.expires'), hideBelow: 'lg', cell: (k) => <DateText value={k.expires_at} style="date" className="text-muted-foreground" /> },
  ];

  const orders = customer.orders ?? [];
  const subs = customer.subscriptions ?? [];
  const keys = customer.license_keys ?? [];

  return (
    <>
      <PageHeader
        title={customer.name || customer.email}
        description={customer.email}
        back={{ href: '/admin/customers', label: t('customers.detail.back') }}
        badges={customer.test_mode ? <TestModeBadge /> : undefined}
        actions={
          <>
            {can.write && (
              <Button icon={<Pencil className="size-3.5" />} onClick={() => setEditing(true)}>
                {t('common.edit')}
              </Button>
            )}
            {can.write && (
              <Button
                variant="secondary"
                icon={<CreditCard className="size-3.5" />}
                loading={portalPending}
                disabled={!customer.stripe_customer_id}
                title={customer.stripe_customer_id ? undefined : t('customers.portal.noStripe')}
                onClick={openPortal}
              >
                {t('customers.portal.open')}
                <ExternalLink className="size-3 text-muted-foreground" />
              </Button>
            )}
          </>
        }
      />

      {can.write && !customer.stripe_customer_id && <p className="-mt-2 mb-4 text-xs text-muted-foreground">{t('customers.portal.noStripe')}</p>}

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="min-w-0 space-y-4 lg:col-span-2">
          <Card>
            <CardHeader title={t('customers.stats')} />
            <CardBody>
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
                <Stat label={t('customers.stat.revenue')}>
                  <MoneyText amount={customer.total_revenue} currency={currency} />
                </Stat>
                <Stat label={t('customers.stat.mrr')}>
                  <MoneyText amount={customer.mrr} currency={currency} />
                </Stat>
                <Stat label={t('customers.stat.orders')}>{customer.orders_count}</Stat>
                <Stat label={t('customers.stat.subscriptions')}>{customer.subscriptions_count}</Stat>
                <Stat label={t('customers.stat.licenseKeys')}>{customer.license_keys_count}</Stat>
              </div>
            </CardBody>
          </Card>

          <Card>
            <Tabs
              className="px-2"
              ariaLabel={t('customers.title')}
              value={tab}
              onValueChange={(v) => set({ tab: v === 'orders' ? '' : v })}
              items={[
                { value: 'orders', label: t('customers.tab.orders'), count: orders.length },
                { value: 'subscriptions', label: t('customers.tab.subscriptions'), count: subs.length },
                { value: 'keys', label: t('customers.tab.licenseKeys'), count: keys.length },
              ]}
            />
            {tab === 'orders' && (
              <DataTable
                columns={orderColumns}
                rows={orders}
                rowKey={(o) => o.id}
                rowHref={(o) => `/admin/orders/${o.id}`}
                empty={{ title: t('customers.tab.emptyOrders') }}
              />
            )}
            {tab === 'subscriptions' && (
              <DataTable
                columns={subColumns}
                rows={subs}
                rowKey={(s) => s.id}
                rowHref={(s) => `/admin/subscriptions/${s.id}`}
                empty={{ title: t('customers.tab.emptySubscriptions') }}
              />
            )}
            {tab === 'keys' && (
              <DataTable
                columns={keyColumns}
                rows={keys}
                rowKey={(k) => k.id}
                rowHref={(k) => `/admin/license-keys/${k.id}`}
                empty={{ title: t('customers.tab.emptyLicenseKeys') }}
              />
            )}
          </Card>
        </div>

        <Card className="h-fit">
          <CardHeader title={t('customers.profile')} />
          <CardBody>
            <DL>
              <DI label={t('customers.field.name')}>{customer.name}</DI>
              <DI label={t('customers.field.email')}>
                <span className="break-all">{customer.email}</span>
              </DI>
              <DI label={t('customers.field.location')}>{location}</DI>
              <DI label={t('customers.field.status')}>
                <span className="capitalize">{customer.status}</span>
              </DI>
              <DI label={t('customers.field.stripeId')}>
                {customer.stripe_customer_id ? (
                  <span className="flex items-center gap-1">
                    <code className="min-w-0 truncate text-xs" title={customer.stripe_customer_id}>
                      {customer.stripe_customer_id}
                    </code>
                    <CopyButton value={customer.stripe_customer_id} label={t('customers.field.stripeId')} />
                    {stripeUrl && (
                      <a
                        href={stripeUrl}
                        target="_blank"
                        rel="noreferrer"
                        aria-label={t('customers.stripeDashboard')}
                        title={t('customers.stripeDashboard')}
                        className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                      >
                        <ExternalLink className="size-3.5" />
                      </a>
                    )}
                  </span>
                ) : null}
              </DI>
              <DI label={t('common.created')}>
                <DateText value={customer.created_at} />
              </DI>
              <DI label={t('common.updated')}>{formatDate(customer.updated_at, locale)}</DI>
            </DL>
          </CardBody>
        </Card>
      </div>

      {editing && <EditCustomerDialog customer={customer} onClose={() => setEditing(false)} onSaved={() => void mutate()} />}
    </>
  );
}
