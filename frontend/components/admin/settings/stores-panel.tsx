'use client';

import { useState } from 'react';
import { Pencil, Plus, Store as StoreIcon, Trash2 } from 'lucide-react';
import { adminApi, isApiError } from '@/lib/api';
import { errorText } from '@/lib/error-text';
import { formatMoney } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { Paginated, Settings, Store } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import {
  ActionMenu,
  Button,
  Card,
  CardBody,
  CardHeader,
  CopyField,
  DataTable,
  Field,
  MoneyText,
  useConfirmDelete,
  useToast,
  type Column,
} from '@/components/ui';
import { useAdmin } from '../admin-context';
import { StoreFormModal } from './store-form-modal';

function StoreCell({ store }: { store: Store }) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-accent-soft text-xs font-semibold uppercase text-accent-text">
        {store.name.slice(0, 2)}
      </span>
      <div className="min-w-0">
        <p className="truncate font-medium text-fg">{store.name}</p>
        <p className="truncate font-mono text-xs text-subtle">{store.slug}</p>
      </div>
    </div>
  );
}

export function StoresPanel() {
  const { t, locale } = useI18n();
  const toast = useToast();
  const { can, refreshStores } = useAdmin();
  const { data, error, isLoading, mutate } = useApi<Paginated<Store>>('/stores', { per_page: 100 }, { scoped: false });
  const [modal, setModal] = useState<{ store: Store | null } | null>(null);
  const { run } = useAction();
  const confirmDelete = useConfirmDelete();

  const refresh = () => Promise.all([mutate(), refreshStores()]);

  async function remove(store: Store) {
    if (!(await confirmDelete(store.name))) return;
    const res = await run(() => adminApi.delete(`/stores/${store.id}`), { silent: true, success: t('toast.deleted') });
    if (res.ok) await refresh();
    else toast.error(isApiError(res.error) && res.error.status === 409 ? t('settings.stores.deleteConflict') : errorText(res.error, t));
  }

  const columns: Column<Store>[] = [
    { key: 'store', header: t('settings.stores.col.store'), cell: (s) => <StoreCell store={s} /> },
    {
      key: 'domain',
      header: t('settings.stores.col.domain'),
      hideBelow: 'md',
      cell: (s) => (
        <div className="min-w-0 text-xs">
          <p className="truncate text-fg">{s.domain || '—'}</p>
          <p className="truncate text-subtle">{s.support_email || ''}</p>
        </div>
      ),
    },
    {
      key: 'region',
      header: t('settings.stores.col.region'),
      hideBelow: 'lg',
      cell: (s) => (
        <span className="whitespace-nowrap text-xs text-muted">
          {s.country || '—'} · {s.currency}
        </span>
      ),
    },
    {
      key: 'sales',
      header: t('settings.stores.col.sales'),
      align: 'right',
      hideBelow: 'sm',
      cell: (s) => (
        <div className="text-xs tabular-nums">
          <p className="text-fg">{new Intl.NumberFormat(locale).format(s.total_sales)}</p>
          <p className="text-subtle">{t('settings.stores.last30', { value: new Intl.NumberFormat(locale).format(s.thirty_day_sales) })}</p>
        </div>
      ),
    },
    {
      key: 'revenue',
      header: t('settings.stores.col.revenue'),
      align: 'right',
      cell: (s) => (
        <div className="text-xs">
          <MoneyText amount={s.total_revenue} currency={s.currency} className="block text-fg" />
          <span className="block text-subtle">
            {t('settings.stores.last30', { value: formatMoney(s.thirty_day_revenue, s.currency, locale) })}
          </span>
        </div>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      align: 'right',
      className: 'w-10',
      cell: (s) => (
        <ActionMenu
          items={[
            { key: 'edit', label: t('common.edit'), icon: <Pencil />, onSelect: () => setModal({ store: s }) },
            { key: 'delete', label: t('common.delete'), icon: <Trash2 />, tone: 'danger', separatorBefore: true, onSelect: () => remove(s) },
          ]}
        />
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader
          title={t('settings.stores.title')}
          description={t('settings.stores.description')}
          actions={
            can.write && (
              <Button variant="primary" size="sm" icon={<Plus className="size-3.5" />} onClick={() => setModal({ store: null })}>
                {t('settings.stores.new')}
              </Button>
            )
          }
        />
        <DataTable
          columns={columns}
          rows={data?.data}
          rowKey={(s) => s.id}
          loading={isLoading}
          error={error}
          onRetry={() => mutate()}
          skeletonRows={3}
          onRowClick={can.write ? (s) => setModal({ store: s }) : undefined}
          empty={{
            icon: <StoreIcon className="size-5" />,
            title: t('settings.stores.empty.title'),
            description: t('settings.stores.empty.body'),
            action: can.write && (
              <Button variant="primary" size="sm" icon={<Plus className="size-3.5" />} onClick={() => setModal({ store: null })}>
                {t('settings.stores.new')}
              </Button>
            ),
          }}
        />
      </Card>

      <PublicUrls />

      {modal && <StoreFormModal key={modal.store?.id ?? 'new'} store={modal.store} onClose={() => setModal(null)} onSaved={refresh} />}
    </div>
  );
}

function PublicUrls() {
  const { t } = useI18n();
  const { can } = useAdmin();
  const { data } = useApi<Settings>(can.write ? '/settings' : null, undefined, { scoped: false, revalidateOnFocus: false });
  if (!data) return null;
  return (
    <Card>
      <CardHeader title={t('settings.stores.urls.title')} description={t('settings.stores.urls.description')} />
      <CardBody className="grid gap-5 sm:grid-cols-2">
        <Field label={t('settings.stores.urls.publicUrl')}>
          <CopyField value={data.urls.public_url} label={t('settings.stores.urls.publicUrl')} />
        </Field>
        <Field label={t('settings.stores.urls.apiBase')} hint={t('settings.stores.urls.apiBaseHint')}>
          <CopyField value={data.urls.api_base_url} label={t('settings.stores.urls.apiBase')} />
        </Field>
      </CardBody>
    </Card>
  );
}
