'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { Mail, Send } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { useList } from '@/lib/use-api';
import type { EmailLog } from '@/lib/types';
import { Button, Card, CardHeader, DataTable, DateText, FilterSelect, Mono, SearchInput, StatusBadge, Toolbar, usePageTitle, type Column } from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';
import { EmailDrawer } from '@/components/admin/team/log-drawers';
import { TestEmailDialog } from '@/components/admin/team/test-email-dialog';

const BASE_STATUSES = ['sent', 'failed', 'queued'];

export default function EmailLogPage() {
  const { t, te } = useI18n();
  const can = useCan();
  usePageTitle(t('team.email.title'));
  const list = useList<EmailLog>('/email-logs', { filters: ['status'], defaultSort: '-created_at' });
  const [selected, setSelected] = useState<EmailLog | null>(null);
  const [testing, setTesting] = useState(false);

  // Statuses are free-form in the contract: static list plus anything seen in the data / the active filter.
  const statuses = useMemo(() => {
    const set = new Set(BASE_STATUSES);
    if (list.values.status) set.add(list.values.status);
    list.rows?.forEach((r) => set.add(r.status));
    return [...set];
  }, [list.rows, list.values.status]);

  const columns: Column<EmailLog>[] = [
    {
      key: 'when',
      header: t('common.date'),
      cell: (l) => (
        <div className="min-w-0">
          <DateText value={l.created_at} className="text-muted-foreground" />
          <p className="mt-0.5 max-w-44 truncate text-xs sm:hidden">{l.to}</p>
        </div>
      ),
    },
    {
      key: 'to',
      header: t('team.email.col.to'),
      hideBelow: 'sm',
      cell: (l) => (
        <div className="min-w-0">
          <span className="block max-w-48 truncate">{l.to}</span>
          <span className="block max-w-48 truncate text-xs text-muted-foreground xl:hidden">{l.subject}</span>
        </div>
      ),
    },
    {
      key: 'subject',
      header: t('team.email.col.subject'),
      hideBelow: 'xl',
      cell: (l) => (
        <div className="min-w-0">
          <span className="block max-w-72 truncate text-muted-foreground">{l.subject}</span>
          <span className="flex max-w-72 items-center gap-2 text-xs">
            {l.template && <Mono className="shrink-0 text-[11px]">{l.template}</Mono>}
            {l.error && (
              <span className="truncate text-danger-text" title={l.error}>
                {l.error}
              </span>
            )}
          </span>
        </div>
      ),
    },
    { key: 'status', header: t('common.status'), cell: (l) => <StatusBadge status={l.status} /> },
    {
      key: 'order',
      header: t('team.email.col.order'),
      hideBelow: 'xl',
      cell: (l) =>
        l.order_id ? (
          <Link href={`/admin/orders/${l.order_id}`} className="text-primary hover:underline">
            #{l.order_id}
          </Link>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
  ];

  return (
    <>
      <Card>
        <CardHeader
          title={t('team.email.title')}
          description={t('team.email.description')}
          actions={
            can.write && (
              <Button icon={<Send className="size-4" />} onClick={() => setTesting(true)}>
                {t('team.email.sendTest')}
              </Button>
            )
          }
        />
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('team.email.searchPlaceholder')} />
          <FilterSelect
            aria-label={t('team.email.filterStatus')}
            value={list.values.status}
            onChange={(v) => list.setFilter('status', v)}
            allLabel={t('common.status')}
            options={statuses.map((s) => ({ value: s, label: te(`status.${s}`, s) }))}
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
          empty={{ icon: <Mail className="size-5" />, title: t('team.email.empty'), description: t('team.email.emptyDesc') }}
        />
      </Card>
      <EmailDrawer log={selected} onClose={() => setSelected(null)} />
      {testing && (
        <TestEmailDialog
          onClose={() => setTesting(false)}
          onSent={() => {
            setTesting(false);
            void list.mutate();
          }}
        />
      )}
    </>
  );
}
