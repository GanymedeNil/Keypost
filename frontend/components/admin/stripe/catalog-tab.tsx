'use client';

import { useState } from 'react';
import { Download, RefreshCw } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { formatMoney } from '@/lib/format';
import type { StripeCatalogPrice, StripeCatalogProduct } from '@/lib/types';
import { useApi } from '@/lib/use-api';
import { cn } from '@/lib/utils';
import { Badge, Button, ButtonLink, Card, Checkbox, DataTable, ErrorState, Toolbar, type Column } from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';
import { ImportModal } from './import-modal';

function usePriceLabel() {
  const { t, te, locale } = useI18n();
  return (p: StripeCatalogPrice): string => {
    if (p.unit_amount === null) return t('stripe.catalog.customAmount');
    const amount = formatMoney(p.unit_amount, p.currency, locale);
    if (!p.recurring) return amount;
    const { interval, interval_count: n } = p.recurring;
    return n > 1
      ? t('stripe.price.recurringN', { amount, count: n, unit: te(`stripe.intervalPlural.${interval}`, interval) })
      : t('stripe.price.recurring', { amount, unit: te(`stripe.interval.${interval}`, interval) });
  };
}

function MappingBadge({ variantId }: { variantId: number | null }) {
  const { t } = useI18n();
  return variantId === null ? <Badge>{t('stripe.catalog.unmapped')}</Badge> : <Badge tone="accent">{t('stripe.catalog.mapped', { id: variantId })}</Badge>;
}

export function CatalogTab() {
  const { t } = useI18n();
  const can = useCan();
  const priceLabel = usePriceLabel();
  const { data, error, isLoading, isValidating, mutate } = useApi<StripeCatalogProduct[]>('/stripe/catalog', undefined, { scoped: false, revalidateOnFocus: false });
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [importing, setImporting] = useState<'selected' | 'all' | null>(null);

  const unmapped = (data ?? []).filter((p) => p.mapped_variant_id === null);
  const unmappedIds = new Set(unmapped.map((p) => p.id));
  // Derive the effective selection: drop ids that are gone or got mapped meanwhile.
  const selected = [...picked].filter((id) => unmappedIds.has(id));
  const allSelected = unmapped.length > 0 && selected.length === unmapped.length;

  const toggle = (id: string, on: boolean) =>
    setPicked((cur) => {
      const next = new Set(cur);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const columns: Column<StripeCatalogProduct>[] = [
    ...(can.write
      ? [
          {
            key: 'select',
            header: (
              <Checkbox
                checked={allSelected}
                disabled={unmapped.length === 0}
                onCheckedChange={(on) => setPicked(on ? new Set(unmapped.map((p) => p.id)) : new Set())}
                aria-label={t('stripe.catalog.selectAll')}
              />
            ),
            className: 'w-10',
            cell: (p: StripeCatalogProduct) =>
              p.mapped_variant_id === null ? (
                <Checkbox
                  checked={selected.includes(p.id)}
                  onCheckedChange={(on) => toggle(p.id, on)}
                  aria-label={t('stripe.catalog.select', { name: p.name })}
                />
              ) : null,
          },
        ]
      : []),
    {
      key: 'product',
      header: t('stripe.catalog.col.product'),
      cell: (p) => (
        <div className="min-w-0 max-w-md">
          <p className="truncate font-medium">{p.name}</p>
          <p className="font-mono text-[11px] text-muted-foreground">{p.id}</p>
          {p.description && <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{p.description}</p>}
          <div className="mt-1.5 sm:hidden">
            <MappingBadge variantId={p.mapped_variant_id} />
          </div>
        </div>
      ),
    },
    {
      key: 'prices',
      header: t('stripe.catalog.col.prices'),
      cell: (p) =>
        p.prices.length === 0 ? (
          <span className="text-xs text-muted-foreground">{t('stripe.catalog.noPrices')}</span>
        ) : (
          <ul className="space-y-0.5">
            {p.prices.map((pr) => (
              <li key={pr.id} className={cn('whitespace-nowrap text-xs tabular-nums', pr.active ? 'text-foreground' : 'text-muted-foreground')} title={pr.id}>
                <span className={cn(!pr.active && 'line-through')}>{priceLabel(pr)}</span>
                {!pr.active && <span className="ml-1.5">({t('stripe.catalog.inactivePrice')})</span>}
              </li>
            ))}
          </ul>
        ),
    },
    {
      key: 'status',
      header: t('stripe.catalog.col.status'),
      hideBelow: 'sm',
      cell: (p) => <Badge dot tone={p.active ? 'success' : 'neutral'}>{p.active ? t('status.active') : t('stripe.catalog.inactive')}</Badge>,
    },
    {
      key: 'mapping',
      header: t('stripe.catalog.col.mapping'),
      hideBelow: 'sm',
      cell: (p) => <MappingBadge variantId={p.mapped_variant_id} />,
    },
  ];

  if (error) {
    return (
      <Card>
        <ErrorState
          error={error}
          title={t('stripe.catalog.loadFailed')}
          onRetry={() => mutate()}
        />
        <div className="flex justify-center pb-8">
          <ButtonLink href="/admin/settings/stripe" size="sm">
            {t('stripe.catalog.openSettings')}
          </ButtonLink>
        </div>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <Toolbar
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <Button icon={<RefreshCw className={cn('size-3.5', isValidating && 'animate-spin-slow')} />} onClick={() => mutate()} disabled={isValidating}>
                {t('common.refresh')}
              </Button>
              {can.write && (
                <>
                  <Button icon={<Download className="size-3.5" />} disabled={selected.length === 0} onClick={() => setImporting('selected')}>
                    {t('stripe.catalog.importSelected', { count: selected.length })}
                  </Button>
                  <Button variant="primary" disabled={unmapped.length === 0} onClick={() => setImporting('all')}>
                    {t('stripe.catalog.importAll')}
                  </Button>
                </>
              )}
            </div>
          }
        >
          <p className="text-xs text-muted-foreground">{t('stripe.catalog.description')}</p>
        </Toolbar>
        <DataTable
          columns={columns}
          rows={data}
          rowKey={(p) => p.id}
          loading={isLoading}
          error={error}
          onRetry={() => mutate()}
          empty={{ title: t('stripe.catalog.empty'), description: t('stripe.catalog.emptyDesc') }}
        />
        {data && data.length > 0 && unmapped.length === 0 && <p className="border-t border-border px-4 py-2.5 text-xs text-muted-foreground">{t('stripe.catalog.allImported')}</p>}
      </Card>
      {importing && (
        <ImportModal
          productIds={importing === 'selected' ? selected : null}
          count={importing === 'selected' ? selected.length : unmapped.length}
          onClose={() => setImporting(null)}
          onImported={() => {
            setImporting(null);
            setPicked(new Set());
            void mutate();
          }}
        />
      )}
    </>
  );
}
