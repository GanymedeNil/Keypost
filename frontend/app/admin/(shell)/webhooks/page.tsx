'use client';

import { useState } from 'react';
import { ChevronDown, ListChecks, Lock, LockOpen, Plus, Send, Pencil, Trash2 } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { Delivery, Webhook } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useList } from '@/lib/use-api';
import {
  ActionMenu,
  Badge,
  Button,
  Card,
  DataTable,
  DateText,
  DropdownContent,
  DropdownMenu,
  DropdownTrigger,
  SearchInput,
  TestModeBadge,
  Toolbar,
  useConfirmDelete,
  type Column,
} from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';
import { TestResultModal } from '@/components/admin/webhooks/test-result-modal';
import { WebhookFormModal } from '@/components/admin/webhooks/webhook-form-modal';

function EventsChip({ events }: { events: string[] }) {
  const { t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-accent px-2 py-0.5 text-xs font-medium text-muted-foreground hover:text-foreground"
          aria-label={events.length === 1 ? t('webhooks.eventsCountOne') : t('webhooks.eventsCount', { count: events.length })}
        >
          {events.length === 1 ? t('webhooks.eventsCountOne') : t('webhooks.eventsCount', { count: events.length })}
          <ChevronDown className="size-3" />
        </button>
      </DropdownTrigger>
      <DropdownContent align="start" className="max-h-72 min-w-56 overflow-y-auto p-2">
        <ul className="space-y-1">
          {events.map((e) => (
            <li key={e} className="font-mono text-xs text-foreground">
              {e}
            </li>
          ))}
        </ul>
      </DropdownContent>
    </DropdownMenu>
  );
}

function StatusPill({ disabled }: { disabled: boolean }) {
  const { t } = useI18n();
  return (
    <Badge dot tone={disabled ? 'neutral' : 'success'}>
      {disabled ? t('webhooks.disabled') : t('webhooks.active')}
    </Badge>
  );
}

export default function WebhooksPage() {
  const { t } = useI18n();
  const can = useCan();
  const confirmDelete = useConfirmDelete();
  const { run } = useAction();
  const list = useList<Webhook>('/webhooks', {});
  const [editing, setEditing] = useState<Webhook | 'new' | null>(null);
  const [testing, setTesting] = useState<number | null>(null);
  const [result, setResult] = useState<Delivery | null>(null);

  async function sendTest(w: Webhook) {
    setTesting(w.id);
    const res = await run(() => adminApi.post<Delivery>(`/webhooks/${w.id}/test`));
    setTesting(null);
    if (res.ok) {
      setResult(res.data);
      await Promise.all([list.mutate(), invalidate('/webhook-deliveries')]);
    }
  }

  async function remove(w: Webhook) {
    if (!(await confirmDelete(w.url, t('webhooks.deleteBody')))) return;
    const res = await run(() => adminApi.delete(`/webhooks/${w.id}`), { success: t('webhooks.deleted') });
    if (res.ok) await Promise.all([list.mutate(), invalidate('/webhook-deliveries')]);
  }

  const columns: Column<Webhook>[] = [
    {
      key: 'url',
      header: t('webhooks.col.endpoint'),
      cell: (w) => (
        <div className="flex min-w-0 max-w-[14rem] flex-col gap-1 xl:max-w-sm">
          {can.write ? (
            <button type="button" onClick={() => setEditing(w)} className="truncate text-left font-mono text-xs font-medium text-foreground hover:text-primary hover:underline" title={w.url}>
              {w.url}
            </button>
          ) : (
            <span className="truncate font-mono text-xs font-medium" title={w.url}>
              {w.url}
            </span>
          )}
          <div className="flex flex-wrap items-center gap-1.5 md:hidden">
            <EventsChip events={w.events} />
            <StatusPill disabled={w.disabled} />
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            {w.test_mode && <TestModeBadge />}
            <span
              className="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
              title={w.secret_set ? t('webhooks.secretSet') : t('webhooks.secretMissing')}
            >
              {w.secret_set ? <Lock className="size-3 text-success" /> : <LockOpen className="size-3 text-warning" />}
              {w.secret_set ? t('webhooks.secretSet') : t('webhooks.secretMissing')}
            </span>
          </div>
        </div>
      ),
    },
    { key: 'events', header: t('webhooks.col.events'), hideBelow: 'md', cell: (w) => <EventsChip events={w.events} /> },
    { key: 'status', header: t('webhooks.col.status'), hideBelow: 'md', cell: (w) => <StatusPill disabled={w.disabled} /> },
    { key: 'last_sent', header: t('webhooks.col.lastSent'), hideBelow: 'xl', cell: (w) => <DateText value={w.last_sent_at} relative className="text-muted-foreground" /> },
    {
      key: 'stats',
      header: t('webhooks.col.last24h'),
      hideBelow: 'md',
      cell: (w) => (
        <span className="inline-flex items-center gap-3 text-xs tabular-nums">
          <span className="text-success-text" title={t('webhooks.stat.success', { count: w.stats.success_24h })}>
            ✓ {w.stats.success_24h}
          </span>
          <span className={w.stats.failed_24h > 0 ? 'text-danger-text' : 'text-muted-foreground'} title={t('webhooks.stat.failed', { count: w.stats.failed_24h })}>
            ✕ {w.stats.failed_24h}
          </span>
        </span>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      align: 'right',
      cell: (w) => (
        <div className="flex items-center justify-end gap-1">
          {can.write && (
            <Button size="sm" variant="ghost" className="hidden 2xl:inline-flex" icon={<Send className="size-3.5" />} loading={testing === w.id} onClick={() => sendTest(w)}>
              {t('webhooks.sendTest')}
            </Button>
          )}
          <ActionMenu
            items={[
              { key: 'edit', label: t('common.edit'), icon: <Pencil />, onSelect: () => setEditing(w), hidden: !can.write },
              { key: 'test', label: t('webhooks.sendTest'), icon: <Send />, onSelect: () => sendTest(w), hidden: !can.write },
              { key: 'deliveries', label: t('webhooks.viewDeliveries'), icon: <ListChecks />, href: `/admin/webhooks/deliveries?webhook_id=${w.id}` },
              { key: 'delete', label: t('common.delete'), icon: <Trash2 />, tone: 'danger', onSelect: () => remove(w), hidden: !can.write, separatorBefore: true },
            ]}
          />
        </div>
      ),
    },
  ];

  return (
    <>
      <Card>
        <Toolbar
          actions={
            can.write && (
              <Button variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setEditing('new')}>
                {t('webhooks.new')}
              </Button>
            )
          }
        >
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('webhooks.searchPlaceholder')} />
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(w) => w.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          empty={{
            title: t('webhooks.empty.title'),
            description: t('webhooks.empty.desc'),
            action: can.write ? (
              <Button variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setEditing('new')}>
                {t('webhooks.new')}
              </Button>
            ) : undefined,
          }}
        />
      </Card>
      {editing && (
        <WebhookFormModal
          key={editing === 'new' ? 'new' : editing.id}
          webhook={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void list.mutate();
          }}
        />
      )}
      {result && <TestResultModal delivery={result} onClose={() => setResult(null)} />}
    </>
  );
}
