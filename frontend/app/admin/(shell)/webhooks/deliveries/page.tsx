'use client';

import { useI18n } from '@/lib/i18n/provider';
import type { Delivery, Paginated, Webhook } from '@/lib/types';
import { useApi, useList } from '@/lib/use-api';
import { useUrlState } from '@/lib/url-state';
import { Button, Card, DataTable, DateText, FilterSelect, StatusBadge, Toolbar, type Column, type FilterOption } from '@/components/kit';
import { DeliveryDrawer } from '@/components/admin/webhooks/delivery-drawer';
import { useWebhookEvents } from '@/components/admin/webhooks/use-webhook-events';
import { cn } from '@/lib/utils';

const STATUSES = ['pending', 'success', 'failed'] as const;

function trimUrl(url: string): string {
  const s = url.replace(/^https?:\/\//, '');
  return s.length > 52 ? `${s.slice(0, 51)}…` : s;
}

export default function DeliveriesPage() {
  const { t, te } = useI18n();
  const list = useList<Delivery>('/webhook-deliveries', { filters: ['webhook_id', 'status', 'event_name'] });
  const webhooks = useApi<Paginated<Webhook>>('/webhooks', { per_page: 100 });
  const events = useWebhookEvents();
  const url = useUrlState();

  const selected = Number(list.values.webhook_id) || null;
  const openId = Number(url.get('delivery')) || null;

  const webhookOptions: FilterOption[] = (webhooks.data?.data ?? []).map((w) => ({ value: String(w.id), label: trimUrl(w.url) }));
  if (selected && !webhookOptions.some((o) => o.value === String(selected))) webhookOptions.push({ value: String(selected), label: `#${selected}` });
  const eventOptions: FilterOption[] = (events.data ?? []).map((e) => ({ value: e, label: e }));
  const statusOptions: FilterOption[] = STATUSES.map((s) => ({ value: s, label: te(`status.${s}`, s) }));

  const columns: Column<Delivery>[] = [
    {
      key: 'event',
      header: t('webhooks.deliveries.col.event'),
      cell: (d) => (
        <div className="min-w-0">
          <p className="font-mono text-xs font-medium">{d.event_name}</p>
          {d.resource_type && (
            <p className="font-mono text-[11px] text-muted-foreground">
              {d.resource_type}
              {d.resource_id !== null ? ` #${d.resource_id}` : ''}
            </p>
          )}
          <DateText value={d.created_at} relative className="text-[11px] text-muted-foreground sm:hidden" />
        </div>
      ),
    },
    {
      key: 'endpoint',
      header: t('webhooks.deliveries.col.endpoint'),
      hideBelow: 'md',
      cell: (d) => (
        <span className="block max-w-xs truncate font-mono text-xs text-muted-foreground" title={d.webhook_url}>
          {trimUrl(d.webhook_url)}
        </span>
      ),
    },
    { key: 'status', header: t('common.status'), cell: (d) => <StatusBadge status={d.status} /> },
    {
      key: 'code',
      header: t('webhooks.deliveries.col.response'),
      hideBelow: 'sm',
      cell: (d) => (
        <span
          className={cn(
            'font-mono text-xs tabular-nums',
            d.last_status_code === null ? 'text-muted-foreground' : d.last_status_code >= 200 && d.last_status_code < 300 ? 'text-success-text' : 'text-danger-text',
          )}
        >
          {d.last_status_code ?? '—'}
        </span>
      ),
    },
    { key: 'attempts', header: t('webhooks.deliveries.col.attempts'), hideBelow: 'lg', align: 'right', cell: (d) => <span className="tabular-nums">{d.attempts}</span> },
    {
      key: 'duration',
      header: t('webhooks.deliveries.col.duration'),
      hideBelow: 'lg',
      align: 'right',
      cell: (d) => <span className="tabular-nums text-muted-foreground">{d.duration_ms === null ? '—' : t('webhooks.durationMs', { ms: d.duration_ms })}</span>,
    },
    { key: 'created', header: t('webhooks.deliveries.col.created'), hideBelow: 'sm', cell: (d) => <DateText value={d.created_at} relative className="text-muted-foreground" /> },
  ];

  return (
    <>
      <Card>
        <Toolbar>
          <FilterSelect
            aria-label={t('webhooks.deliveries.filter.webhook')}
            value={list.values.webhook_id}
            onChange={(v) => list.setFilter('webhook_id', v)}
            options={webhookOptions}
            allLabel={t('webhooks.deliveries.filter.allWebhooks')}
            className="sm:w-60"
          />
          <FilterSelect
            aria-label={t('webhooks.deliveries.filter.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            options={statusOptions}
            allLabel={t('webhooks.deliveries.filter.allStatuses')}
          />
          <FilterSelect
            aria-label={t('webhooks.deliveries.filter.event')}
            value={list.values.event_name}
            onChange={(v) => list.setFilter('event_name', v)}
            options={eventOptions}
            allLabel={t('webhooks.deliveries.filter.allEvents')}
            className="sm:w-56"
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
          rowKey={(d) => d.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          onRowClick={(d) => url.set({ delivery: d.id }, { keepPage: true })}
          empty={{ title: t('webhooks.deliveries.empty'), description: t('webhooks.deliveries.emptyDesc') }}
        />
      </Card>
      <DeliveryDrawer id={openId} onClose={() => url.set({ delivery: null }, { keepPage: true })} />
    </>
  );
}
