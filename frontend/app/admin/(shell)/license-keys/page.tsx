'use client';

import { useState } from 'react';
import Link from 'next/link';
import { KeyRound, Plus } from 'lucide-react';
import type { LicenseKey, LicenseStatus } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { useList } from '@/lib/use-api';
import { useCan } from '@/components/admin/admin-context';
import { FilterChip } from '@/components/admin/licenses/filter-chip';
import { IssueLicenseDialog } from '@/components/admin/licenses/issue-license-dialog';
import {
  Button,
  Card,
  CopyButton,
  DataTable,
  DateText,
  FilterSelect,
  PageHeader,
  SearchInput,
  StatusBadge,
  Toolbar,
  type Column,
} from '@/components/kit';

const STATUSES: LicenseStatus[] = ['active', 'inactive', 'expired', 'disabled'];
const ID_FILTERS = ['order_id', 'customer_id', 'variant_id', 'product_id'] as const;

export default function LicenseKeysPage() {
  const { t, te } = useI18n();
  const can = useCan();
  const [issuing, setIssuing] = useState(false);
  const list = useList<LicenseKey>('/license-keys', {
    filters: ['status', ...ID_FILTERS],
    defaultSort: '-created_at',
  });

  const columns: Column<LicenseKey>[] = [
    {
      key: 'key',
      header: t('licenses.col.key'),
      cell: (k) => (
        <div className="flex items-center gap-1">
          <Link href={`/admin/license-keys/${k.id}`} className="whitespace-nowrap font-mono text-[13px] font-medium text-foreground hover:text-primary">
            {k.key_short}
          </Link>
          <CopyButton value={k.key} label={t('licenses.copyKey')} />
        </div>
      ),
    },
    {
      key: 'product',
      header: t('licenses.col.product'),
      hideBelow: 'md',
      cell: (k) => (
        <div className="min-w-0">
          <span className="block truncate">{k.product_name}</span>
          <span className="block truncate text-xs text-muted-foreground">{k.variant_name}</span>
        </div>
      ),
    },
    {
      key: 'customer',
      header: t('licenses.col.customer'),
      hideBelow: 'sm',
      cell: (k) => (
        <div className="min-w-0">
          <span className="block max-w-56 truncate">{k.user_email}</span>
          {k.user_name && <span className="block max-w-56 truncate text-xs text-muted-foreground">{k.user_name}</span>}
        </div>
      ),
    },
    { key: 'status', header: t('licenses.col.status'), cell: (k) => <StatusBadge status={k.status} label={te(`status.${k.status}`, k.status_formatted)} /> },
    {
      key: 'activations',
      header: t('licenses.col.activations'),
      align: 'right',
      cell: (k) => (
        <span className="whitespace-nowrap tabular-nums">
          {k.instances_count} <span className="text-muted-foreground">/ {k.activation_limit ?? '∞'}</span>
        </span>
      ),
    },
    {
      key: 'expires',
      header: t('licenses.col.expires'),
      hideBelow: 'lg',
      cell: (k) => (k.expires_at ? <DateText value={k.expires_at} style="date" /> : <span className="whitespace-nowrap text-muted-foreground">{t('licenses.noExpiry')}</span>),
    },
    {
      key: 'created',
      header: t('licenses.col.created'),
      sortKey: 'created_at',
      hideBelow: 'xl',
      cell: (k) => <DateText value={k.created_at} style="date" className="text-muted-foreground" />,
    },
  ];

  const chipLabels = {
    order_id: 'licenses.filter.order',
    customer_id: 'licenses.filter.customer',
    variant_id: 'licenses.filter.variant',
    product_id: 'licenses.filter.product',
  } as const;

  return (
    <>
      <PageHeader
        title={t('licenses.title')}
        description={t('licenses.description')}
        actions={
          can.write && (
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setIssuing(true)}>
              {t('licenses.issue.button')}
            </Button>
          )
        }
      />
      <Card>
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('licenses.searchPlaceholder')} />
          <FilterSelect
            aria-label={t('licenses.filter.status')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            allLabel={`${t('common.status')}: ${t('common.all')}`}
            options={STATUSES.map((s) => ({ value: s, label: te(`status.${s}`, s) }))}
          />
          {ID_FILTERS.filter((f) => list.values[f]).map((f) => (
            <FilterChip
              key={f}
              label={t(chipLabels[f], { id: list.values[f] })}
              removeLabel={t('licenses.filter.remove')}
              onRemove={() => list.setFilter(f, '')}
            />
          ))}
          {list.hasFilters && (
            <Button variant="ghost" size="sm" onClick={list.clearFilters}>
              {t('common.clearFilters')}
            </Button>
          )}
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(k) => k.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          sort={list.sort}
          onSortChange={list.setSort}
          rowHref={(k) => `/admin/license-keys/${k.id}`}
          filtered={list.hasFilters}
          empty={{
            icon: <KeyRound className="size-5" />,
            title: t('licenses.empty.title'),
            description: t('licenses.empty.description'),
            action: can.write ? (
              <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setIssuing(true)}>
                {t('licenses.issue.button')}
              </Button>
            ) : undefined,
          }}
        />
      </Card>
      {issuing && <IssueLicenseDialog defaultCustomerId={list.values.customer_id} onClose={() => setIssuing(false)} />}
    </>
  );
}
