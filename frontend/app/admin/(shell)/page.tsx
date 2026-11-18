'use client';

import { Boxes, CreditCard, DollarSign, Download, KeyRound, RefreshCcw, RotateCcw, ShoppingCart, Users } from 'lucide-react';
import { Button, ErrorState, PageHeader, Tabs } from '@/components/kit';
import { HealthCard } from '@/components/admin/dashboard/health-card';
import { KpiCard } from '@/components/admin/dashboard/kpi-card';
import { RecentEvents, RecentOrders } from '@/components/admin/dashboard/recent';
import { SalesChart } from '@/components/admin/dashboard/sales-chart';
import { useI18n } from '@/lib/i18n/provider';
import { formatNumber } from '@/lib/format';
import { MoneyText } from '@/components/kit';
import type { Dashboard } from '@/lib/types';
import { percentChange } from '@/lib/utils';
import { useUrlState } from '@/lib/url-state';
import { useApi } from '@/lib/use-api';

const RANGES = [7, 30, 90] as const;

export default function DashboardPage() {
  const { t, locale } = useI18n();
  const { get, set } = useUrlState();
  const raw = Number(get('days'));
  const days = (RANGES as readonly number[]).includes(raw) ? raw : 30;
  const { data, error, isLoading, isValidating, mutate } = useApi<Dashboard>('/dashboard', { days }, { storeScoped: true, refreshInterval: 60_000, keepPreviousData: true });

  const cur = data?.currency ?? 'USD';
  const totals = data?.totals;
  const vs = t('dashboard.kpi.vsPrevious', { days });
  const loading = isLoading && !data;

  return (
    <>
      <PageHeader
        title={t('dashboard.title')}
        description={t('dashboard.subtitle')}
        actions={
          <>
            <Tabs
              ariaLabel={t('dashboard.range')}
              value={String(days)}
              onValueChange={(v) => set({ days: v === '30' ? null : v })}
              className="border-b-0"
              items={RANGES.map((d) => ({ value: String(d), label: t('dashboard.days', { days: d }) }))}
            />
            <Button variant="secondary" size="icon" aria-label={t('common.refresh')} onClick={() => mutate()} disabled={isValidating}>
              <RefreshCcw className={isValidating ? 'size-4 animate-spin-slow' : 'size-4'} />
            </Button>
          </>
        }
      />
      {error && !data ? (
        <ErrorState error={error} onRetry={() => mutate()} />
      ) : (
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 min-[480px]:grid-cols-2 xl:grid-cols-4">
            <KpiCard
              loading={loading}
              icon={<DollarSign />}
              label={t('dashboard.kpi.revenue')}
              value={<MoneyText amount={totals?.revenue} currency={cur} />}
              delta={data ? percentChange(data.totals.revenue, data.previous.revenue) : undefined}
              deltaLabel={vs}
            />
            <KpiCard
              loading={loading}
              icon={<ShoppingCart />}
              label={t('dashboard.kpi.orders')}
              value={formatNumber(totals?.orders, locale)}
              delta={data ? percentChange(data.totals.orders, data.previous.orders) : undefined}
              deltaLabel={vs}
            />
            <KpiCard
              loading={loading}
              icon={<Users />}
              label={t('dashboard.kpi.customers')}
              value={formatNumber(totals?.customers, locale)}
              sub={t('dashboard.kpi.allTime')}
            />
            <KpiCard
              loading={loading}
              icon={<RotateCcw />}
              label={t('dashboard.kpi.refunded')}
              value={<MoneyText amount={totals?.refunded} currency={cur} />}
              sub={t('dashboard.days', { days })}
            />
            <KpiCard
              loading={loading}
              icon={<CreditCard />}
              label={t('dashboard.kpi.subscriptions')}
              value={formatNumber(totals?.active_subscriptions, locale)}
            />
            <KpiCard
              loading={loading}
              icon={<Boxes />}
              label={t('dashboard.kpi.mrr')}
              value={<MoneyText amount={totals?.mrr} currency={cur} />}
              sub={t('dashboard.kpi.monthlyRecurring')}
            />
            <KpiCard
              loading={loading}
              icon={<KeyRound />}
              label={t('dashboard.kpi.licenseKeys')}
              value={formatNumber(totals?.license_keys, locale)}
              sub={t('dashboard.kpi.activeKeys', { count: formatNumber(totals?.active_license_keys, locale) })}
            />
            <KpiCard
              loading={loading}
              icon={<Download />}
              label={t('dashboard.kpi.downloads')}
              value={formatNumber(totals?.downloads, locale)}
              sub={t('dashboard.kpi.allTime')}
            />
          </div>

          <div className="grid gap-4 xl:grid-cols-3">
            <div className="xl:col-span-2">
              <SalesChart data={data} days={days} loading={loading} />
            </div>
            <HealthCard health={data?.health} loading={loading} />
          </div>

          <div className="grid gap-4 xl:grid-cols-3">
            <div className="xl:col-span-2">
              <RecentOrders orders={data?.recent_orders} loading={loading} />
            </div>
            <RecentEvents events={data?.recent_events} loading={loading} />
          </div>
        </div>
      )}
    </>
  );
}
