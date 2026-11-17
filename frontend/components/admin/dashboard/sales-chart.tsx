'use client';

import { useId, useMemo, useState, type PointerEvent } from 'react';
import { Card, CardHeader, EmptyState, Tabs } from '@/components/ui';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate, formatMoney, formatNumber } from '@/lib/format';
import type { Dashboard } from '@/lib/types';
import { cn } from '@/lib/utils';

type Metric = 'revenue' | 'orders';

const W = 800;
const H = 220;
const PAD_T = 12;

function niceMax(v: number): number {
  if (v <= 0) return 1;
  const exp = 10 ** Math.floor(Math.log10(v));
  const f = v / exp;
  const n = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
  return n * exp;
}

/** Hand-rolled SVG chart: area for revenue, bars for orders. Hover/keyboard shows a tooltip. */
export function SalesChart({ data, days, loading }: { data: Dashboard | undefined; days: number; loading: boolean }) {
  const { t, locale } = useI18n();
  const gid = useId().replace(/:/g, '');
  const [metric, setMetric] = useState<Metric>('revenue');
  const [hover, setHover] = useState<number | null>(null);
  const currency = data?.currency ?? 'USD';
  const series = useMemo(() => data?.series ?? [], [data]);

  const values = useMemo(() => series.map((s) => (metric === 'revenue' ? s.revenue : s.orders)), [series, metric]);
  const total = values.reduce((a, b) => a + b, 0);
  const max = niceMax(Math.max(0, ...values));
  const n = series.length;
  const x = (i: number) => (n <= 1 ? W / 2 : (i / (n - 1)) * W);
  const y = (v: number) => PAD_T + (1 - v / max) * (H - PAD_T);
  const fmt = (v: number) => (metric === 'revenue' ? formatMoney(v, currency, locale, { compact: true }) : formatNumber(v, locale));

  const line = values.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
  const area = n > 0 ? `${line} L${x(n - 1).toFixed(1)},${H} L${x(0).toFixed(1)},${H} Z` : '';
  const barW = n > 0 ? Math.max(2, (W / n) * 0.62) : 0;

  function onMove(e: PointerEvent<HTMLDivElement>) {
    if (n === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
    setHover(metric === 'orders' ? Math.min(n - 1, Math.floor(ratio * n)) : Math.round(ratio * (n - 1)));
  }

  const point = hover !== null ? series[hover] : undefined;
  const leftPct = hover !== null && n > 0 ? (metric === 'orders' ? ((hover + 0.5) / n) * 100 : n <= 1 ? 50 : (hover / (n - 1)) * 100) : 0;
  const ticks = [0, 0.5, 1];
  const labelIdx = n > 0 ? [0, Math.floor((n - 1) / 3), Math.floor(((n - 1) * 2) / 3), n - 1] : [];

  return (
    <Card className="h-full">
      <CardHeader
        title={t('dashboard.chart.title')}
        description={
          <span className="tabular-nums">
            {t(metric === 'revenue' ? 'dashboard.chart.revenue' : 'dashboard.chart.orders')}: {fmt(total)} · {t('dashboard.days', { days })}
          </span>
        }
        actions={
          <Tabs
            ariaLabel={t('dashboard.chart.title')}
            value={metric}
            onValueChange={(v) => setMetric(v as Metric)}
            className="border-b-0"
            items={[
              { value: 'revenue', label: t('dashboard.chart.revenue') },
              { value: 'orders', label: t('dashboard.chart.orders') },
            ]}
          />
        }
      />
      <div className="p-4 pt-3">
        {loading && !data ? (
          <div className="h-60 animate-shimmer rounded-lg bg-surface-3" />
        ) : n === 0 || total === 0 ? (
          <EmptyState title={t('dashboard.chart.empty')} className="py-16" />
        ) : (
          <div className="relative flex gap-2">
            <div className="flex h-[220px] w-12 shrink-0 flex-col justify-between pb-0 text-right text-[11px] tabular-nums text-subtle" aria-hidden>
              {[...ticks].reverse().map((tk) => (
                <span key={tk} className="leading-none">
                  {fmt(max * tk)}
                </span>
              ))}
            </div>
            <div className="min-w-0 flex-1">
              <div
                className="relative touch-pan-y"
                onPointerMove={onMove}
                onPointerLeave={() => setHover(null)}
                role="img"
                aria-label={t('dashboard.chart.aria', {
                  metric: t(metric === 'revenue' ? 'dashboard.chart.revenue' : 'dashboard.chart.orders'),
                  days,
                  total: fmt(total),
                })}
              >
                <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="block h-[220px] w-full overflow-visible">
                  <defs>
                    <linearGradient id={`g${gid}`} x1="0" x2="0" y1="0" y2="1">
                      <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.28" />
                      <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.02" />
                    </linearGradient>
                  </defs>
                  {ticks.map((tk) => (
                    <line key={tk} x1="0" x2={W} y1={y(max * tk)} y2={y(max * tk)} stroke="var(--line)" strokeWidth="1" vectorEffect="non-scaling-stroke" strokeDasharray={tk === 0 ? undefined : '3 4'} />
                  ))}
                  {metric === 'revenue' ? (
                    <>
                      <path d={area} fill={`url(#g${gid})`} />
                      <path d={line} fill="none" stroke="var(--accent)" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
                    </>
                  ) : (
                    values.map((v, i) => (
                      <rect
                        key={i}
                        x={x(i) - barW / 2 + (n > 1 ? 0 : 0)}
                        y={y(v)}
                        width={barW}
                        height={Math.max(0, H - y(v))}
                        rx="2"
                        fill="var(--accent)"
                        opacity={hover === null || hover === i ? 1 : 0.45}
                      />
                    ))
                  )}
                  {hover !== null && metric === 'revenue' && (
                    <line x1={x(hover)} x2={x(hover)} y1="0" y2={H} stroke="var(--line-strong)" strokeWidth="1" vectorEffect="non-scaling-stroke" />
                  )}
                </svg>
                {hover !== null && metric === 'revenue' && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-accent bg-surface"
                    style={{ left: `${leftPct}%`, top: `${(y(values[hover]) / H) * 100}%` }}
                  />
                )}
                {point && (
                  <div
                    className={cn(
                      'pointer-events-none absolute top-0 z-10 w-44 rounded-lg border border-line bg-surface p-2.5 text-xs shadow-[var(--shadow-pop)]',
                      leftPct > 60 ? '-translate-x-[105%]' : 'translate-x-3',
                    )}
                    style={{ left: `${leftPct}%` }}
                  >
                    <p className="mb-1 font-medium text-fg">{formatDate(point.date, locale, 'date')}</p>
                    <dl className="space-y-0.5 tabular-nums text-muted">
                      <div className="flex justify-between gap-2">
                        <dt>{t('dashboard.chart.revenue')}</dt>
                        <dd className="text-fg">{formatMoney(point.revenue, currency, locale)}</dd>
                      </div>
                      <div className="flex justify-between gap-2">
                        <dt>{t('dashboard.chart.orders')}</dt>
                        <dd className="text-fg">{formatNumber(point.orders, locale)}</dd>
                      </div>
                      <div className="flex justify-between gap-2">
                        <dt>{t('dashboard.chart.refunds')}</dt>
                        <dd className="text-fg">{formatMoney(point.refunds, currency, locale)}</dd>
                      </div>
                    </dl>
                  </div>
                )}
              </div>
              <div className="mt-1.5 flex justify-between text-[11px] tabular-nums text-subtle" aria-hidden>
                {labelIdx.map((i, k) => (
                  <span key={`${i}-${k}`}>{series[i] ? formatDate(series[i].date, locale, 'short') : ''}</span>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}
