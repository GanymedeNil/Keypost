'use client';

import Link from 'next/link';
import { Download, ShoppingCart } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { dateToRange, formatMoney } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { Order, OrderStatus } from '@/lib/types';
import { useUrlState } from '@/lib/url-state';
import { useList } from '@/lib/use-api';
import {
  Button,
  ButtonAnchor,
  Card,
  DataTable,
  FilterSelect,
  Input,
  MoneyText,
  PageHeader,
  SearchInput,
  TestModeBadge,
  Toolbar,
  type Column,
  StatusBadge,
  DateText,
} from '@/components/ui';

const STATUSES: OrderStatus[] = ['paid', 'pending', 'partial_refund', 'refunded', 'failed', 'fraudulent'];

export default function OrdersPage() {
  const { t, te, locale } = useI18n();
  const { get } = useUrlState();
  const from = get('from');
  const to = get('to');
  const list = useList<Order>('/orders', {
    filters: ['status'],
    defaultSort: '-created_at',
    query: { created_from: dateToRange(from, 'start'), created_to: dateToRange(to, 'end') },
  });
  const filtered = list.hasFilters || !!from || !!to;

  const columns: Column<Order>[] = [
    {
      key: 'order',
      header: t('orders.col.order'),
      cell: (o) => (
        <div className="min-w-0">
          <Link href={`/admin/orders/${o.id}`} className="font-medium text-fg hover:text-accent-text">
            #{o.order_number}
          </Link>
          <p className="max-w-40 truncate text-xs text-subtle sm:hidden">{o.user_name || o.user_email}</p>
          <p className="hidden max-w-40 truncate font-mono text-[11px] text-subtle sm:block" title={o.identifier}>
            {o.identifier}
          </p>
        </div>
      ),
    },
    {
      key: 'customer',
      header: t('orders.col.customer'),
      hideBelow: 'sm',
      cell: (o) => (
        <div className="min-w-0 max-w-56">
          <p className="truncate">{o.user_name || '—'}</p>
          <p className="truncate text-xs text-subtle">{o.user_email}</p>
        </div>
      ),
    },
    {
      key: 'item',
      header: t('orders.col.item'),
      hideBelow: 'lg',
      cell: (o) =>
        o.first_item ? (
          <div className="min-w-0 max-w-64">
            <p className="truncate">{o.first_item.product_name}</p>
            <p className="truncate text-xs text-subtle">
              {o.first_item.variant_name}
              {o.first_item.quantity > 1 ? ` × ${o.first_item.quantity}` : ''}
            </p>
          </div>
        ) : (
          <span className="text-subtle">—</span>
        ),
    },
    {
      key: 'total',
      header: t('orders.col.total'),
      align: 'right',
      sortKey: 'total',
      cell: (o) => (
        <div>
          <MoneyText amount={o.total} currency={o.currency} className="font-medium" />
          {o.refunded_amount > 0 && (
            <p className="text-xs text-subtle">{t('orders.refundedAmount', { amount: formatMoney(o.refunded_amount, o.currency, locale) })}</p>
          )}
        </div>
      ),
    },
    {
      key: 'status',
      header: t('common.status'),
      cell: (o) => (
        <div className="flex flex-wrap items-center gap-1">
          <StatusBadge status={o.status} />
          {o.test_mode && <TestModeBadge />}
        </div>
      ),
    },
    {
      key: 'source',
      header: t('orders.col.source'),
      hideBelow: 'xl',
      cell: (o) => <span className="text-muted">{te(`source.${o.source}`, o.source)}</span>,
    },
    {
      key: 'created',
      header: t('common.created'),
      sortKey: 'created_at',
      hideBelow: 'md',
      cell: (o) => <DateText value={o.created_at} className="text-muted" />,
    },
  ];

  return (
    <>
      <PageHeader
        title={t('orders.title')}
        description={t('orders.description')}
        actions={
          <ButtonAnchor href={adminApi.url('/orders/export.csv', list.exportQuery)} icon={<Download className="size-3.5" />}>
            {t('common.exportCsv')}
          </ButtonAnchor>
        }
      />
      <Card>
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('orders.search')} className="sm:w-72" />
          <FilterSelect
            aria-label={t('orders.filter.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            allLabel={`${t('orders.filter.status')}: ${t('common.all')}`}
            options={STATUSES.map((s) => ({ value: s, label: te(`status.${s}`, s) }))}
          />
          <div className="flex items-center gap-1.5">
            <Input
              type="date"
              aria-label={t('common.dateFrom')}
              title={t('common.dateFrom')}
              value={from}
              max={to || undefined}
              onChange={(e) => list.set({ from: e.target.value })}
              className="w-36"
            />
            <span className="text-xs text-subtle">–</span>
            <Input
              type="date"
              aria-label={t('common.dateTo')}
              title={t('common.dateTo')}
              value={to}
              min={from || undefined}
              onChange={(e) => list.set({ to: e.target.value })}
              className="w-36"
            />
          </div>
          {filtered && (
            <Button variant="ghost" size="sm" onClick={() => list.set({ q: '', status: '', from: '', to: '' })}>
              {t('common.clearFilters')}
            </Button>
          )}
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(o) => o.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          sort={list.sort}
          onSortChange={list.setSort}
          rowHref={(o) => `/admin/orders/${o.id}`}
          filtered={filtered}
          empty={{ title: t('orders.empty.title'), description: t('orders.empty.description'), icon: <ShoppingCart className="size-5" /> }}
        />
      </Card>
    </>
  );
}
