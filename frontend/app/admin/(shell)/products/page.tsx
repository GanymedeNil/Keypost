'use client';

import Link from 'next/link';
import { useState } from 'react';
import { Package, Plus } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { formatMoney } from '@/lib/format';
import { useList } from '@/lib/use-api';
import type { Product } from '@/lib/types';
import { Badge, Button, Card, DateText, FilterSelect, PageHeader, SearchInput, StatusBadge, TestModeBadge, Toolbar, type Column } from '@/components/ui';
import { DataTable } from '@/components/ui';
import { useAdmin, useCan } from '@/components/admin/admin-context';
import { NewProductDialog } from '@/components/admin/products/new-product-dialog';
import { Thumb } from '@/components/admin/products/thumb';

export default function ProductsPage() {
  const { t, locale } = useI18n();
  const can = useCan();
  const { stores } = useAdmin();
  const [creating, setCreating] = useState(false);
  const list = useList<Product>('/products', { filters: ['status'], defaultSort: '-created_at' });

  const currencyOf = (p: Product) => stores.find((s) => s.id === p.store_id)?.currency ?? 'USD';

  const columns: Column<Product>[] = [
    {
      key: 'name',
      header: t('common.name'),
      sortKey: 'name',
      cell: (p) => (
        <div className="flex min-w-0 items-center gap-3">
          <Thumb url={p.thumb_url} label={p.name} />
          <div className="min-w-0">
            <Link href={`/admin/products/${p.id}`} className="block truncate font-medium text-fg hover:text-accent-text">
              {p.name}
            </Link>
            <p className="truncate text-xs text-subtle">{p.slug}</p>
          </div>
        </div>
      ),
    },
    { key: 'status', header: t('common.status'), cell: (p) => <StatusBadge status={p.status} /> },
    { key: 'variants', header: t('products.col.variants'), align: 'right', hideBelow: 'sm', cell: (p) => <span className="tabular-nums">{p.variants_count}</span> },
    {
      key: 'price',
      header: t('products.col.price'),
      hideBelow: 'md',
      cell: (p) => {
        const cur = currencyOf(p);
        if (p.from_price === null || p.from_price === undefined) return <span className="text-subtle">—</span>;
        const from = formatMoney(p.from_price, cur, locale);
        const to = formatMoney(p.to_price ?? p.from_price, cur, locale);
        return <span className="whitespace-nowrap tabular-nums">{from === to ? from : `${from} – ${to}`}</span>;
      },
    },
    {
      key: 'flags',
      header: t('products.col.flags'),
      hideBelow: 'lg',
      cell: (p) => (
        <div className="flex flex-wrap gap-1">
          {p.test_mode && <TestModeBadge />}
          {p.stripe_product_id ? <Badge tone="success">{t('products.stripe.mapped')}</Badge> : <Badge tone="warning">{t('products.stripe.notMapped')}</Badge>}
        </div>
      ),
    },
    { key: 'updated', header: t('common.updated'), sortKey: 'updated_at', hideBelow: 'lg', cell: (p) => <DateText value={p.updated_at} style="date" /> },
  ];

  return (
    <>
      <PageHeader
        title={t('products.title')}
        description={t('products.subtitle')}
        actions={
          can.write && (
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
              {t('products.new.button')}
            </Button>
          )
        }
      />
      <Card>
        <Toolbar
          actions={
            list.hasFilters && (
              <Button size="sm" variant="ghost" onClick={list.clearFilters}>
                {t('common.clearFilters')}
              </Button>
            )
          }
        >
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('products.searchPlaceholder')} />
          <FilterSelect
            aria-label={t('common.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            allLabel={t('products.filter.allStatuses')}
            options={[
              { value: 'draft', label: t('status.draft') },
              { value: 'published', label: t('status.published') },
            ]}
          />
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(p) => p.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          sort={list.sort}
          onSortChange={list.setSort}
          rowHref={(p) => `/admin/products/${p.id}`}
          filtered={list.hasFilters}
          empty={{
            icon: <Package className="size-5" />,
            title: t('products.empty.title'),
            description: t('products.empty.desc'),
            action: can.write ? (
              <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
                {t('products.new.button')}
              </Button>
            ) : undefined,
          }}
        />
      </Card>
      <NewProductDialog open={creating} onOpenChange={setCreating} />
    </>
  );
}
