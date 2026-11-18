'use client';

import { useState } from 'react';
import { ScrollText } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { useList } from '@/lib/use-api';
import type { AuditLog } from '@/lib/types';
import { Card, CardHeader, DataTable, DateText, FilterSelect, Mono, SearchInput, Toolbar, usePageTitle, type Column } from '@/components/kit';
import { AuditDrawer, resourceLabel } from '@/components/admin/team/log-drawers';

const RESOURCE_TYPES = [
  'order',
  'customer',
  'subscription',
  'license_key',
  'product',
  'variant',
  'price',
  'file',
  'webhook',
  'api_key',
  'user',
  'store',
  'settings',
  'import',
];

function compact(details: unknown): string {
  if (details === null || details === undefined || details === '') return '—';
  const s = typeof details === 'string' ? details : JSON.stringify(details);
  return s.length > 60 ? `${s.slice(0, 60)}…` : s;
}

export default function AuditLogPage() {
  const { t } = useI18n();
  usePageTitle(t('team.audit.title'));
  const list = useList<AuditLog>('/audit-logs', { filters: ['action', 'user_email', 'resource_type'], defaultSort: '-created_at' });
  const [selected, setSelected] = useState<AuditLog | null>(null);

  const types = list.values.resource_type && !RESOURCE_TYPES.includes(list.values.resource_type) ? [...RESOURCE_TYPES, list.values.resource_type] : RESOURCE_TYPES;

  const columns: Column<AuditLog>[] = [
    {
      key: 'when',
      header: t('common.date'),
      cell: (l) => (
        <div className="min-w-0">
          <DateText value={l.created_at} className="text-muted-foreground" />
          <div className="mt-1 sm:hidden">
            <Mono className="inline-block max-w-full truncate align-top">{l.action}</Mono>
          </div>
        </div>
      ),
    },
    { key: 'user', header: t('team.audit.col.user'), hideBelow: 'sm', cell: (l) => <span className="block max-w-48 truncate">{l.user_email ?? <span className="text-muted-foreground">{t('team.audit.system')}</span>}</span> },
    { key: 'action', header: t('team.audit.col.action'), hideBelow: 'sm', cell: (l) => <Mono>{l.action}</Mono> },
    { key: 'resource', header: t('team.audit.col.resource'), hideBelow: 'lg', cell: (l) => <span className="text-muted-foreground">{resourceLabel(l)}</span> },
    { key: 'ip', header: t('team.audit.col.ip'), hideBelow: 'xl', cell: (l) => <span className="font-mono text-xs text-muted-foreground">{l.ip ?? '—'}</span> },
    {
      key: 'details',
      header: t('team.audit.col.details'),
      hideBelow: 'xl',
      cell: (l) => <span className="block max-w-72 truncate font-mono text-xs text-muted-foreground">{compact(l.details)}</span>,
    },
  ];

  return (
    <>
      <Card>
        <CardHeader title={t('team.audit.title')} description={t('team.audit.description')} />
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('team.audit.searchPlaceholder')} />
          <div className="grid w-full grid-cols-2 gap-2 sm:contents">
            <SearchInput
              value={list.values.action}
              onChange={(v) => list.setFilter('action', v)}
              placeholder={t('team.audit.filterAction')}
              aria-label={t('team.audit.filterAction')}
              className="sm:w-44"
            />
            <SearchInput
              value={list.values.user_email}
              onChange={(v) => list.setFilter('user_email', v)}
              placeholder={t('team.audit.filterUser')}
              aria-label={t('team.audit.filterUser')}
              className="sm:w-52"
            />
          </div>
          <FilterSelect
            aria-label={t('team.audit.filterResource')}
            value={list.values.resource_type}
            onChange={(v) => list.setFilter('resource_type', v)}
            allLabel={t('team.audit.allResources')}
            options={types.map((r) => ({ value: r, label: r }))}
          />
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(l) => l.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          onRowClick={setSelected}
          empty={{ icon: <ScrollText className="size-5" />, title: t('team.audit.empty'), description: t('team.audit.emptyDesc') }}
        />
      </Card>
      <AuditDrawer log={selected} onClose={() => setSelected(null)} />
    </>
  );
}
