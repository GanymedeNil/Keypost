'use client';

import { cn } from '@/lib/utils';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate, formatMoney, formatRelative } from '@/lib/format';

export function MoneyText({
  amount,
  currency,
  compact,
  className,
}: {
  /** Integer minor units (cents). */
  amount: number | null | undefined;
  currency?: string | null;
  compact?: boolean;
  className?: string;
}) {
  const { locale } = useI18n();
  return <span className={cn('tabular-nums', className)}>{formatMoney(amount, currency, locale, { compact })}</span>;
}

export function DateText({
  value,
  style = 'datetime',
  relative,
  className,
}: {
  value: string | null | undefined;
  style?: 'date' | 'datetime' | 'time' | 'short';
  /** Show "3 hours ago" with the absolute date in the tooltip. */
  relative?: boolean;
  className?: string;
}) {
  const { locale } = useI18n();
  if (!value) return <span className={cn('text-subtle', className)}>—</span>;
  const abs = formatDate(value, locale, 'datetime');
  return (
    <time dateTime={value} title={abs} suppressHydrationWarning className={cn('whitespace-nowrap tabular-nums', className)}>
      {relative ? formatRelative(value, locale) : formatDate(value, locale, style)}
    </time>
  );
}
