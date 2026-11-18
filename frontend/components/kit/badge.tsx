'use client';

import type { ReactNode } from 'react';
import { cn, humanize } from '@/lib/utils';
import { Badge as UiBadge } from '@/components/ui/badge';
import { useI18n } from '@/lib/i18n/provider';

export type Tone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger' | 'info';

const tones: Record<Tone, string> = {
  neutral: '',
  accent: '',
  success: 'border-transparent bg-success-soft text-success-text',
  warning: 'border-transparent bg-warning-soft text-warning-text',
  danger: 'border-transparent bg-danger-soft text-danger-text',
  info: 'border-transparent bg-info-soft text-info-text',
};

const dots: Record<Tone, string> = {
  neutral: 'bg-muted-foreground',
  accent: 'bg-primary',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
};

export function Badge({
  tone = 'neutral',
  dot,
  className,
  children,
}: {
  tone?: Tone;
  dot?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <UiBadge variant={tone === 'accent' ? 'default' : tone === 'neutral' ? 'secondary' : 'outline'} className={cn('gap-1.5', tones[tone], className)}>
      {dot && <span aria-hidden className={cn('size-1.5 rounded-full', dots[tone])} />}
      {children}
    </UiBadge>
  );
}

const statusTone: Record<string, Tone> = {
  paid: 'success',
  active: 'success',
  success: 'success',
  processed: 'success',
  completed: 'success',
  published: 'success',
  sent: 'success',
  on_trial: 'info',
  running: 'info',
  received: 'info',
  queued: 'info',
  pending: 'warning',
  paused: 'warning',
  partial_refund: 'warning',
  past_due: 'warning',
  failed: 'danger',
  fraudulent: 'danger',
  unpaid: 'danger',
  expired: 'danger',
  disabled: 'danger',
  refunded: 'neutral',
  cancelled: 'neutral',
  canceled: 'neutral',
  inactive: 'neutral',
  draft: 'neutral',
  ignored: 'neutral',
  void: 'neutral',
};

/** Colour-coded status pill with an i18n label (`status.<value>`), falls back to a humanised value. */
export function StatusBadge({ status, label, className }: { status: string | null | undefined; label?: string; className?: string }) {
  const { te } = useI18n();
  if (!status) return <span className="text-muted-foreground">—</span>;
  return (
    <Badge dot tone={statusTone[status] ?? 'neutral'} className={className}>
      {label ?? te(`status.${status}`, humanize(status))}
    </Badge>
  );
}

export function TestModeBadge({ className }: { className?: string }) {
  const { te } = useI18n();
  return (
    <Badge tone="warning" className={className}>
      {te('common.testMode', 'Test mode')}
    </Badge>
  );
}
