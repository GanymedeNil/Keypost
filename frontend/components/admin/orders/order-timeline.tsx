'use client';

import { useMemo } from 'react';
import { Mail, Webhook } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import type { Delivery, EmailLog } from '@/lib/types';
import { DateText, StatusBadge } from '@/components/kit';

type Entry =
  | { kind: 'email'; id: string; at: string; email: EmailLog }
  | { kind: 'webhook'; id: string; at: string; delivery: Delivery };

/** Emails and outbound webhook deliveries for one order, newest first. */
export function OrderTimeline({ emails, deliveries }: { emails: EmailLog[]; deliveries: Delivery[] }) {
  const { t } = useI18n();
  const entries = useMemo<Entry[]>(() => {
    const all: Entry[] = [
      ...emails.map((email): Entry => ({ kind: 'email', id: `e${email.id}`, at: email.created_at, email })),
      ...deliveries.map((delivery): Entry => ({ kind: 'webhook', id: `w${delivery.id}`, at: delivery.delivered_at ?? delivery.created_at, delivery })),
    ];
    return all.sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
  }, [emails, deliveries]);

  if (entries.length === 0) return <p className="px-4 py-8 text-center text-sm text-muted-foreground">{t('orders.timeline.empty')}</p>;

  return (
    <ol className="relative px-4 py-3">
      <span aria-hidden className="absolute bottom-6 left-[2.15rem] top-6 w-px bg-border" />
      {entries.map((entry) => (
        <li key={entry.id} className="relative flex gap-3 py-2.5">
          <span
            className={`z-10 grid size-7 shrink-0 place-items-center rounded-full border border-border bg-card ${entry.kind === 'email' ? 'text-info-text' : 'text-primary'}`}
          >
            {entry.kind === 'email' ? <Mail className="size-3.5" /> : <Webhook className="size-3.5" />}
          </span>
          <div className="min-w-0 flex-1">
            {entry.kind === 'email' ? <EmailRow email={entry.email} /> : <DeliveryRow delivery={entry.delivery} />}
            <p className="mt-0.5 text-xs text-muted-foreground">
              <DateText value={entry.at} />
            </p>
          </div>
        </li>
      ))}
    </ol>
  );
}

function EmailRow({ email }: { email: EmailLog }) {
  const { t } = useI18n();
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-sm font-medium">{t('orders.timeline.email')}</span>
        <StatusBadge status={email.status} />
      </div>
      <p className="truncate text-sm text-muted-foreground" title={email.subject}>
        {email.subject}
      </p>
      <p className="truncate text-xs text-muted-foreground">{t('orders.timeline.to', { to: email.to })}</p>
      {email.error && <p className="mt-0.5 break-words text-xs text-danger-text">{email.error}</p>}
    </>
  );
}

function DeliveryRow({ delivery }: { delivery: Delivery }) {
  const { t } = useI18n();
  const meta = [
    delivery.last_status_code ? t('orders.timeline.http', { code: delivery.last_status_code }) : null,
    delivery.duration_ms !== null ? t('orders.timeline.duration', { ms: delivery.duration_ms }) : null,
    t('orders.timeline.attempts', { count: delivery.attempts }),
  ].filter(Boolean);
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-sm font-medium">{t('orders.timeline.webhook')}</span>
        <code className="rounded bg-accent px-1.5 py-0.5 text-xs">{delivery.event_name}</code>
        <StatusBadge status={delivery.status} />
      </div>
      <p className="truncate text-sm text-muted-foreground" title={delivery.webhook_url}>
        {delivery.webhook_url}
      </p>
      <p className="text-xs text-muted-foreground">{meta.join(' · ')}</p>
      {delivery.last_error && <p className="mt-0.5 break-words text-xs text-danger-text">{delivery.last_error}</p>}
    </>
  );
}
