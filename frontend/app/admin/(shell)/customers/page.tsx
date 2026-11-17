'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { Plus, Users } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import type { Customer } from '@/lib/types';
import { useList } from '@/lib/use-api';
import { useCan } from '@/components/admin/admin-context';
import { CreateCustomerDialog } from '@/components/admin/customers/customer-dialog';
import { useStoreCurrency } from '@/components/admin/customers/store-currency';
import { Badge, Button, Card, DataTable, DateText, MoneyText, PageHeader, SearchInput, TestModeBadge, Toolbar, type Column } from '@/components/ui';

export default function CustomersPage() {
  const { t } = useI18n();
  const can = useCan();
  const router = useRouter();
  const storeCurrency = useStoreCurrency();
  const [creating, setCreating] = useState(false);
  const list = useList<Customer>('/customers', { filters: ['country'], defaultSort: '-created_at' });

  const columns: Column<Customer>[] = [
    {
      key: 'customer',
      header: t('customers.col.customer'),
      sortKey: 'name',
      cell: (c) => (
        <div className="min-w-0 max-w-64">
          <div className="flex items-center gap-1.5">
            <Link href={`/admin/customers/${c.id}`} className="truncate font-medium hover:text-accent-text">
              {c.name || c.email}
            </Link>
            {c.test_mode && <TestModeBadge />}
          </div>
          <p className="truncate text-xs text-subtle">{c.email}</p>
        </div>
      ),
    },
    { key: 'country', header: t('common.country'), hideBelow: 'sm', cell: (c) => c.country || <span className="text-subtle">—</span> },
    { key: 'orders', header: t('customers.col.orders'), align: 'right', hideBelow: 'sm', cell: (c) => <span className="tabular-nums">{c.orders_count}</span> },
    {
      key: 'subs',
      header: t('customers.col.subscriptions'),
      align: 'right',
      hideBelow: 'lg',
      cell: (c) => <span className="tabular-nums">{c.subscriptions_count}</span>,
    },
    {
      key: 'keys',
      header: t('customers.col.licenseKeys'),
      align: 'right',
      hideBelow: 'lg',
      cell: (c) => <span className="tabular-nums">{c.license_keys_count}</span>,
    },
    {
      key: 'revenue',
      header: t('customers.col.revenue'),
      align: 'right',
      sortKey: 'total_revenue',
      cell: (c) => <MoneyText amount={c.total_revenue} currency={storeCurrency(c.store_id)} className="font-medium" />,
    },
    {
      key: 'mrr',
      header: t('customers.col.mrr'),
      align: 'right',
      hideBelow: 'md',
      cell: (c) => (c.mrr > 0 ? <MoneyText amount={c.mrr} currency={storeCurrency(c.store_id)} /> : <span className="text-subtle">—</span>),
    },
    {
      key: 'stripe',
      header: t('customers.col.stripe'),
      hideBelow: 'xl',
      cell: (c) =>
        c.stripe_customer_id ? <Badge tone="success">{t('customers.stripe.linked')}</Badge> : <Badge>{t('customers.stripe.notLinked')}</Badge>,
    },
    { key: 'created', header: t('common.created'), sortKey: 'created_at', hideBelow: 'xl', cell: (c) => <DateText value={c.created_at} style="date" className="text-muted" /> },
  ];

  return (
    <>
      <PageHeader
        title={t('customers.title')}
        description={t('customers.description')}
        actions={
          can.write && (
            <Button variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setCreating(true)}>
              {t('customers.new')}
            </Button>
          )
        }
      />
      <Card>
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('customers.search')} className="sm:w-72" />
          <SearchInput
            value={list.values.country}
            onChange={(v) => list.setFilter('country', v.trim().toUpperCase())}
            placeholder={t('customers.countryFilter')}
            aria-label={t('customers.countryFilter')}
            className="sm:w-44"
          />
          {list.hasFilters && (
            <Button variant="ghost" size="sm" onClick={list.clearFilters}>
              {t('common.clearFilters')}
            </Button>
          )}
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(c) => c.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          sort={list.sort}
          onSortChange={list.setSort}
          rowHref={(c) => `/admin/customers/${c.id}`}
          filtered={list.hasFilters}
          empty={{
            title: t('customers.empty.title'),
            description: t('customers.empty.description'),
            icon: <Users className="size-5" />,
            action: can.write ? (
              <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
                {t('customers.new')}
              </Button>
            ) : undefined,
          }}
        />
      </Card>
      {creating && <CreateCustomerDialog onClose={() => setCreating(false)} onCreated={(c) => router.push(`/admin/customers/${c.id}`)} />}
    </>
  );
}
