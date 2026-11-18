'use client';

import Link from 'next/link';
import { ChevronRight, Download, KeyRound } from 'lucide-react';
import type { StorefrontOrderSummary, StorefrontSubscriptionSummary } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate } from '@/lib/format';
import { Badge, Card, DateText, StatusBadge } from '@/components/kit';
import { ManageBilling } from './billing';

export function OrderCard({ order }: { order: StorefrontOrderSummary }) {
  const { t } = useI18n();
  const shown = order.items.slice(0, 2);
  const more = order.items.length - shown.length;
  return (
    <li>
      <Link
        href={`/my-orders/${encodeURIComponent(order.identifier)}`}
        aria-label={t('storefront.me.openOrder', { number: order.order_number })}
        className="group block rounded-xl border border-border bg-card p-4 shadow-card transition-colors hover:border-input hover:bg-muted/60 focus-visible:outline-2 focus-visible:outline-ring sm:p-5"
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold">{t('storefront.me.orderNumber', { number: order.order_number })}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              <DateText value={order.created_at} style="date" />
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <div className="text-right">
              <p className="text-sm font-semibold tabular-nums">{order.total_formatted}</p>
              <StatusBadge status={order.status} className="mt-1" />
            </div>
            <ChevronRight className="size-5 text-muted-foreground transition-transform group-hover:translate-x-0.5" aria-hidden />
          </div>
        </div>
        {shown.length > 0 && (
          <ul className="mt-3 space-y-0.5 text-sm text-muted-foreground">
            {shown.map((item, i) => (
              <li key={i} className="truncate">
                <span className="text-foreground">{item.product_name}</span>
                {item.variant_name && <span> — {item.variant_name}</span>}
              </li>
            ))}
            {more > 0 && <li className="text-xs text-muted-foreground">{t('storefront.me.moreItems', { count: more })}</li>}
          </ul>
        )}
        {(order.license_key_count > 0 || order.file_count > 0) && (
          <div className="mt-3 flex flex-wrap gap-1.5">
            {order.license_key_count > 0 && (
              <Badge tone="accent">
                <KeyRound className="size-3" aria-hidden />
                {t(order.license_key_count === 1 ? 'storefront.chip.keys.one' : 'storefront.chip.keys.other', { count: order.license_key_count })}
              </Badge>
            )}
            {order.file_count > 0 && (
              <Badge tone="info">
                <Download className="size-3" aria-hidden />
                {t(order.file_count === 1 ? 'storefront.chip.files.one' : 'storefront.chip.files.other', { count: order.file_count })}
              </Badge>
            )}
          </div>
        )}
      </Link>
    </li>
  );
}

export function SubscriptionCard({ sub }: { sub: StorefrontSubscriptionSummary }) {
  const { t, locale } = useI18n();
  const ended = sub.status === 'cancelled' || sub.status === 'expired';
  let note: string | null = null;
  if (sub.ends_at && (ended || !sub.renews_at)) note = t('storefront.sub.endsOn', { date: formatDate(sub.ends_at, locale, 'date') });
  else if (sub.renews_at) note = t('storefront.sub.renewsOn', { date: formatDate(sub.renews_at, locale, 'date') });
  return (
    <li>
      <Card className="p-4 sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold">{sub.product_name}</p>
            {sub.variant_name && <p className="text-sm text-muted-foreground">{sub.variant_name}</p>}
            {note && <p className="mt-1 text-xs text-muted-foreground">{note}</p>}
          </div>
          <StatusBadge status={sub.status} />
        </div>
        <ManageBilling href={sub.billing_url} className="mt-3 w-full sm:w-auto" />
      </Card>
    </li>
  );
}
