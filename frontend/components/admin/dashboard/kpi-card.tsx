'use client';

import type { ReactNode } from 'react';
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-react';
import { Card } from '@/components/ui';
import { useI18n } from '@/lib/i18n/provider';
import { formatPercent } from '@/lib/format';
import { cn } from '@/lib/utils';

/** `delta`: percent change vs the previous period (null = no baseline, "new"). `undefined` hides the delta row. */
export function KpiCard({
  label,
  value,
  icon,
  delta,
  deltaLabel,
  sub,
  invertDelta,
  loading,
}: {
  label: string;
  value: ReactNode;
  icon: ReactNode;
  delta?: number | null;
  deltaLabel?: string;
  sub?: ReactNode;
  /** For metrics where lower is better (refunds). */
  invertDelta?: boolean;
  loading?: boolean;
}) {
  const { t, locale } = useI18n();
  let deltaNode: ReactNode = null;
  if (delta !== undefined && !loading) {
    const up = delta !== null && delta > 0;
    const flat = delta === 0;
    const good = invertDelta ? !up : up;
    const Icon = flat ? Minus : up ? ArrowUpRight : ArrowDownRight;
    deltaNode = (
      <p className="mt-1 flex flex-wrap items-center gap-x-1.5 text-xs text-subtle">
        <span
          className={cn(
            'inline-flex items-center gap-0.5 rounded px-1 py-0.5 font-medium',
            flat || delta === null ? 'bg-surface-3 text-muted' : good ? 'bg-success-soft text-success-text' : 'bg-danger-soft text-danger-text',
          )}
        >
          <Icon className="size-3" />
          {delta === null ? t('dashboard.kpi.new') : flat ? t('dashboard.kpi.noChange') : formatPercent(delta, locale)}
        </span>
        {deltaLabel}
      </p>
    );
  }
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-medium text-muted">{label}</p>
        <span className="grid size-7 place-items-center rounded-md bg-accent-soft text-accent-text [&_svg]:size-4">{icon}</span>
      </div>
      <div className="mt-2 text-2xl font-semibold tracking-tight tabular-nums">
        {loading ? <span className="block h-8 w-28 animate-shimmer rounded bg-surface-3" /> : value}
      </div>
      {deltaNode}
      {sub && !loading && <p className="mt-1 text-xs text-subtle">{sub}</p>}
    </Card>
  );
}
