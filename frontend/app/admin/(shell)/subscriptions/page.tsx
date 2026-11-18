'use client';

import Link from 'next/link';
import { Repeat } from 'lucide-react';
import type { Subscription, SubscriptionStatus } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate } from '@/lib/format';
import { useList } from '@/lib/use-api';
import {
  Badge,
  Button,
  Card,
  DataTable,
  DateText,
  FilterSelect,
  PageHeader,
  SearchInput,
  Toolbar,
  type Column,
  StatusBadge,
} from '@/components/kit';
import { FilterChip } from '@/components/admin/licenses/filter-chip';

const STATUSES: SubscriptionStatus[] = ['on_trial', 'active', 'paused', 'past_due', 'unpaid', 'cancelled', 'expired'];

export default function SubscriptionsPage() {
  const { t, te, locale } = useI18n();
  const list = useList<Subscription>('/subscriptions', {
    filters: ['status', 'customer_id', 'product_id'],
    defaultSort: '-created_at',
  });

  const columns: Column<Subscription>[] = [
    {
      key: 'customer',
      header: t('subscriptions.col.customer'),
      cell: (s) => (
        <div className="min-w-0">
          <Link href={`/admin/subscriptions/${s.id}`} className="block truncate font-medium text-foreground hover:text-primary">
            {s.user_name || s.user_email}
          </Link>
          <span className="block truncate text-xs text-muted-foreground">{s.user_email}</span>
        </div>
      ),
    },
    {
      key: 'plan',
      header: t('subscriptions.col.plan'),
      hideBelow: 'md',
      cell: (s) => (
        <div className="min-w-0">
          <span className="block truncate">{s.product_name}</span>
          <span className="block truncate text-xs text-muted-foreground">{s.variant_name}</span>
        </div>
      ),
    },
    {
      key: 'status',
      header: t('subscriptions.col.status'),
      cell: (s) => (
        <div className="flex flex-col items-start gap-1">
          <StatusBadge status={s.status} label={te(`status.${s.status}`, s.status_formatted)} />
          {s.status === 'on_trial' && s.trial_ends_at && (
            <span className="whitespace-nowrap text-xs text-muted-foreground">{t('subscriptions.trialEnds', { date: formatDate(s.trial_ends_at, locale, 'date') })}</span>
          )}
          {s.cancelled && s.status !== 'cancelled' && s.ends_at && (
            <span className="whitespace-nowrap text-xs text-muted-foreground">{t('subscriptions.endsOn', { date: formatDate(s.ends_at, locale, 'date') })}</span>
          )}
        </div>
      ),
    },
    {
      key: 'renews',
      header: t('subscriptions.col.renews'),
      hideBelow: 'sm',
      cell: (s) => <DateText value={s.renews_at ?? s.ends_at} style="date" />,
    },
    {
      key: 'card',
      header: t('subscriptions.col.card'),
      hideBelow: 'lg',
      cell: (s) =>
        s.card_last_four ? (
          <span className="whitespace-nowrap text-muted-foreground">
            <span className="capitalize">{s.card_brand ?? ''}</span> ··{s.card_last_four}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      key: 'quantity',
      header: t('subscriptions.col.quantity'),
      align: 'right',
      hideBelow: 'xl',
      cell: (s) => <span className="tabular-nums">{s.quantity}</span>,
    },
    {
      key: 'stripe',
      header: t('subscriptions.col.stripe'),
      hideBelow: 'lg',
      cell: (s) =>
        s.stripe_subscription_id ? (
          <Badge tone="success">{t('subscriptions.stripeLinked')}</Badge>
        ) : (
          <Badge tone="warning">{t('subscriptions.stripeNotLinked')}</Badge>
        ),
    },
    {
      key: 'created',
      header: t('subscriptions.col.created'),
      sortKey: 'created_at',
      hideBelow: 'xl',
      cell: (s) => <DateText value={s.created_at} style="date" className="text-muted-foreground" />,
    },
  ];

  return (
    <>
      <PageHeader title={t('subscriptions.title')} description={t('subscriptions.description')} />
      <Card>
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('subscriptions.searchPlaceholder')} className="sm:w-72" />
          <FilterSelect
            aria-label={t('subscriptions.filter.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            allLabel={`${t('common.status')}: ${t('common.all')}`}
            options={STATUSES.map((s) => ({ value: s, label: te(`status.${s}`, s) }))}
          />
          {list.values.customer_id && (
            <FilterChip
              label={t('subscriptions.filter.customer', { id: list.values.customer_id })}
              removeLabel={t('subscriptions.filter.remove')}
              onRemove={() => list.setFilter('customer_id', '')}
            />
          )}
          {list.values.product_id && (
            <FilterChip
              label={t('subscriptions.filter.product', { id: list.values.product_id })}
              removeLabel={t('subscriptions.filter.remove')}
              onRemove={() => list.setFilter('product_id', '')}
            />
          )}
          {list.hasFilters && (
            <Button variant="ghost" size="sm" onClick={list.clearFilters}>
              {t('common.clearFilters')}
            </Button>
          )}
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(s) => s.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          sort={list.sort}
          onSortChange={list.setSort}
          rowHref={(s) => `/admin/subscriptions/${s.id}`}
          filtered={list.hasFilters}
          empty={{ icon: <Repeat className="size-5" />, title: t('subscriptions.empty.title'), description: t('subscriptions.empty.description') }}
        />
      </Card>
    </>
  );
}
