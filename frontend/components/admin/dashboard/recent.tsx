'use client';

import Link from 'next/link';
import { Download, Webhook, CreditCard } from 'lucide-react';
import { Card, CardHeader, DateText, EmptyState, MoneyText, Skeleton, StatusBadge, TestModeBadge } from '@/components/ui';
import { useI18n } from '@/lib/i18n/provider';
import type { DashboardEvent, Order } from '@/lib/types';

export function RecentOrders({ orders, loading }: { orders: Order[] | undefined; loading: boolean }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader
        title={t('dashboard.recentOrders')}
        actions={
          <Link href="/admin/orders" className="text-xs font-medium text-accent-text hover:underline">
            {t('dashboard.viewAll')}
          </Link>
        }
      />
      {loading && !orders ? (
        <div className="space-y-3 p-4">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      ) : !orders || orders.length === 0 ? (
        <EmptyState title={t('dashboard.recentOrdersEmpty')} description={t('dashboard.recentOrdersEmptyHint')} />
      ) : (
        <ul className="divide-y divide-line">
          {orders.map((o) => (
            <li key={o.id}>
              <Link href={`/admin/orders/${o.id}`} className="flex items-center gap-3 px-4 py-2.5 hover:bg-surface-2/70">
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2 truncate text-sm font-medium">
                    #{o.order_number}
                    <span className="truncate font-normal text-muted">{o.user_name || o.user_email}</span>
                    {o.test_mode && <TestModeBadge />}
                  </p>
                  <p className="truncate text-xs text-subtle">
                    {o.first_item ? `${o.first_item.product_name} — ${o.first_item.variant_name}` : o.user_email}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <MoneyText amount={o.total} currency={o.currency} className="text-sm font-medium" />
                  <p className="text-xs text-subtle">
                    <DateText value={o.created_at} relative />
                  </p>
                </div>
                <span className="hidden shrink-0 sm:block">
                  <StatusBadge status={o.status} />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function EventIcon({ kind }: { kind: string }) {
  const cls = 'size-3.5';
  if (kind === 'stripe_event') return <CreditCard className={cls} />;
  if (kind === 'webhook_delivery') return <Webhook className={cls} />;
  return <Download className={cls} />;
}

function eventHref(kind: string): string | undefined {
  if (kind === 'stripe_event') return '/admin/stripe?tab=events';
  if (kind === 'webhook_delivery') return '/admin/webhooks/deliveries';
  if (kind === 'import') return '/admin/imports';
  return undefined;
}

export function RecentEvents({ events, loading }: { events: DashboardEvent[] | undefined; loading: boolean }) {
  const { t, te } = useI18n();
  return (
    <Card>
      <CardHeader title={t('dashboard.recentEvents')} />
      {loading && !events ? (
        <div className="space-y-3 p-4">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-7 w-full" />
          ))}
        </div>
      ) : !events || events.length === 0 ? (
        <EmptyState title={t('dashboard.recentEventsEmpty')} />
      ) : (
        <ul className="divide-y divide-line">
          {events.map((e, i) => {
            const href = eventHref(e.kind);
            const body = (
              <>
                <span className="grid size-7 shrink-0 place-items-center rounded-full bg-surface-3 text-muted">
                  <EventIcon kind={e.kind} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs text-fg">{e.title}</p>
                  <p className="text-xs text-subtle">
                    {te(`dashboard.event.${e.kind}`, e.kind)} · <DateText value={e.at} relative />
                  </p>
                </div>
                <StatusBadge status={e.status} />
              </>
            );
            return (
              <li key={`${e.at}-${i}`}>
                {href ? (
                  <Link href={href} className="flex items-center gap-3 px-4 py-2.5 hover:bg-surface-2/70">
                    {body}
                  </Link>
                ) : (
                  <div className="flex items-center gap-3 px-4 py-2.5">{body}</div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}
