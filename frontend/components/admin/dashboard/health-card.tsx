'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Clock, HardDrive } from 'lucide-react';
import { Badge, Card, CardHeader, Skeleton } from '@/components/ui';
import { useI18n } from '@/lib/i18n/provider';
import type { DashboardHealth } from '@/lib/types';
import { cn } from '@/lib/utils';

interface Check {
  key: string;
  level: 'ok' | 'warn' | 'info';
  title: string;
  hint?: string;
  href?: string;
  cta?: string;
  icon?: ReactNode;
}

function buildChecks(h: DashboardHealth, t: ReturnType<typeof useI18n>['t']): Check[] {
  const checks: Check[] = [];
  checks.push(
    h.stripe_configured
      ? { key: 'stripe', level: 'ok', title: t('dashboard.health.stripeOk') }
      : { key: 'stripe', level: 'warn', title: t('dashboard.health.stripeMissing'), hint: t('dashboard.health.stripeMissingHint'), href: '/admin/settings/stripe', cta: t('dashboard.health.fix') },
  );
  checks.push(
    h.stripe_webhook_secret_set
      ? { key: 'whsec', level: 'ok', title: t('dashboard.health.webhookOk') }
      : { key: 'whsec', level: 'warn', title: t('dashboard.health.webhookMissing'), hint: t('dashboard.health.webhookMissingHint'), href: '/admin/stripe', cta: t('dashboard.health.fix') },
  );
  checks.push(
    h.smtp_configured
      ? { key: 'smtp', level: 'ok', title: t('dashboard.health.smtpOk') }
      : { key: 'smtp', level: 'warn', title: t('dashboard.health.smtpMissing'), hint: t('dashboard.health.smtpMissingHint'), href: '/admin/settings/email', cta: t('dashboard.health.fix') },
  );
  if (h.failed_stripe_events > 0) {
    checks.push({
      key: 'events',
      level: 'warn',
      title: t('dashboard.health.stripeEvents', { count: h.failed_stripe_events }),
      hint: t('dashboard.health.stripeEventsHint'),
      href: '/admin/stripe?tab=events&status=failed',
      cta: t('dashboard.health.view'),
    });
  }
  if (h.failed_webhook_deliveries > 0) {
    checks.push({
      key: 'deliveries',
      level: 'warn',
      title: t('dashboard.health.deliveries', { count: h.failed_webhook_deliveries }),
      hint: t('dashboard.health.deliveriesHint'),
      href: '/admin/webhooks/deliveries?status=failed',
      cta: t('dashboard.health.view'),
    });
  }
  if (h.pending_webhook_deliveries > 0) {
    checks.push({
      key: 'pending',
      level: 'info',
      title: t('dashboard.health.pending', { count: h.pending_webhook_deliveries }),
      href: '/admin/webhooks/deliveries?status=pending',
      cta: t('dashboard.health.view'),
      icon: <Clock className="size-4 text-info" />,
    });
  }
  checks.push({
    key: 'storage',
    level: 'info',
    title: t('dashboard.health.storage', { driver: h.storage_driver }),
    icon: <HardDrive className="size-4 text-subtle" />,
  });
  return checks;
}

export function HealthCard({ health, loading }: { health: DashboardHealth | undefined; loading: boolean }) {
  const { t } = useI18n();
  const checks = health ? buildChecks(health, t) : [];
  const warnings = checks.filter((c) => c.level === 'warn').length;
  return (
    <Card>
      <CardHeader
        title={t('dashboard.health.title')}
        actions={
          health &&
          (warnings > 0 ? (
            <Badge tone="warning" dot>
              {t('dashboard.health.needsAttention', { count: warnings })}
            </Badge>
          ) : (
            <Badge tone="success" dot>
              {t('dashboard.health.allGood')}
            </Badge>
          ))
        }
      />
      <ul className="divide-y divide-line">
        {loading && !health
          ? Array.from({ length: 4 }, (_, i) => (
              <li key={i} className="px-4 py-3">
                <Skeleton className="h-4 w-3/4" />
              </li>
            ))
          : checks.map((c) => (
              <li key={c.key} className={cn('flex items-start gap-3 px-4 py-3', c.level === 'warn' && 'bg-warning-soft/40')}>
                <span className="mt-0.5 shrink-0">
                  {c.icon ??
                    (c.level === 'ok' ? <CheckCircle2 className="size-4 text-success" /> : <AlertTriangle className="size-4 text-warning" />)}
                </span>
                <div className="min-w-0 flex-1">
                  <p className={cn('text-sm', c.level === 'warn' ? 'font-medium text-fg' : 'text-muted')}>{c.title}</p>
                  {c.hint && <p className="mt-0.5 text-xs text-subtle">{c.hint}</p>}
                </div>
                {c.href && (
                  <Link
                    href={c.href}
                    className={cn(
                      'shrink-0 rounded-md px-2 py-1 text-xs font-medium',
                      c.level === 'warn' ? 'bg-surface text-fg ring-1 ring-line-strong hover:bg-surface-2' : 'text-accent-text hover:underline',
                    )}
                  >
                    {c.cta}
                  </Link>
                )}
              </li>
            ))}
      </ul>
    </Card>
  );
}
