'use client';

import { useI18n } from '@/lib/i18n/provider';
import type { StripeEvent, StripeStatus } from '@/lib/types';
import { useUrlState } from '@/lib/url-state';
import { useApi, useList } from '@/lib/use-api';
import { Button, Card, DataTable, DateText, FilterSelect, SearchInput, StatusBadge, Toolbar, type Column, type FilterOption } from '@/components/kit';
import { EventDrawer } from './event-drawer';
import { LivemodeBadge } from './livemode-badge';

const STATUSES = ['received', 'processed', 'ignored', 'failed'] as const;

export function EventsTab() {
  const { t, te } = useI18n();
  const list = useList<StripeEvent>('/stripe-events', { filters: ['status', 'type'] });
  const url = useUrlState();
  // The required event types are a good, always-available source for the type filter.
  const status = useApi<StripeStatus>('/stripe/status', undefined, { scoped: false, revalidateOnFocus: false });

  const openId = Number(url.get('event')) || null;
  const typeOptions: FilterOption[] = (status.data?.required_events ?? []).map((e) => ({ value: e, label: e }));
  const currentType = list.values.type;
  if (currentType && !typeOptions.some((o) => o.value === currentType)) typeOptions.push({ value: currentType, label: currentType });
  const statusOptions: FilterOption[] = STATUSES.map((s) => ({ value: s, label: te(`status.${s}`, s) }));

  const columns: Column<StripeEvent>[] = [
    {
      key: 'type',
      header: t('stripe.events.col.type'),
      cell: (e) => (
        <div className="min-w-0">
          <p className="font-mono text-xs font-medium">{e.type}</p>
          {e.account && <p className="font-mono text-[11px] text-muted-foreground">{e.account}</p>}
          <DateText value={e.received_at} relative className="text-[11px] text-muted-foreground sm:hidden" />
        </div>
      ),
    },
    { key: 'status', header: t('common.status'), cell: (e) => <StatusBadge status={e.status} /> },
    { key: 'mode', header: t('stripe.events.col.mode'), hideBelow: 'lg', cell: (e) => <LivemodeBadge livemode={e.livemode} /> },
    { key: 'attempts', header: t('stripe.events.col.attempts'), hideBelow: 'xl', align: 'right', cell: (e) => <span className="tabular-nums">{e.attempts}</span> },
    {
      key: 'error',
      header: t('stripe.events.col.error'),
      hideBelow: 'xl',
      cell: (e) =>
        e.error ? (
          <span className="block max-w-xs truncate text-xs text-danger-text" title={e.error}>
            {e.error}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    { key: 'received', header: t('stripe.events.col.received'), hideBelow: 'sm', cell: (e) => <DateText value={e.received_at} relative className="text-muted-foreground" /> },
    { key: 'processed', header: t('stripe.events.col.processed'), hideBelow: 'xl', cell: (e) => <DateText value={e.processed_at} relative className="text-muted-foreground" /> },
  ];

  return (
    <>
      <Card>
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('stripe.events.searchPlaceholder')} className="sm:w-60" />
          <FilterSelect
            aria-label={t('stripe.events.filter.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            options={statusOptions}
            allLabel={t('stripe.events.allStatuses')}
          />
          <FilterSelect
            aria-label={t('stripe.events.filter.type')}
            value={list.values.type}
            onChange={(v) => list.setFilter('type', v)}
            options={typeOptions}
            allLabel={t('stripe.events.allTypes')}
            className="sm:w-52"
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
          rowKey={(e) => e.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          onRowClick={(e) => url.set({ event: e.id }, { keepPage: true })}
          empty={{ title: t('stripe.events.empty'), description: t('stripe.events.emptyDesc') }}
        />
      </Card>
      <EventDrawer id={openId} onClose={() => url.set({ event: null }, { keepPage: true })} />
    </>
  );
}
