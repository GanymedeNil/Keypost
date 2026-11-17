'use client';

import { useState } from 'react';
import { KeyRound, Lock, Plus, Trash2 } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useApi, useList } from '@/lib/use-api';
import type { ApiKey, ApiKeyCreated, Settings } from '@/lib/types';
import {
  ActionMenu,
  Badge,
  Button,
  Callout,
  Card,
  CardBody,
  CardHeader,
  CopyButton,
  CopyField,
  DataTable,
  DateText,
  EmptyState,
  SearchInput,
  Toolbar,
  useConfirm,
  usePageTitle,
  type Column,
} from '@/components/ui';
import { useCan } from '@/components/admin/admin-context';
import { CreateKeyDialog, TokenDialog } from '@/components/admin/team/api-key-dialogs';
import { useOrigin } from '@/components/admin/team/use-origin';

type KeyState = 'active' | 'revoked' | 'expired';

function keyState(k: ApiKey, now: number): KeyState {
  if (k.revoked_at) return 'revoked';
  if (k.expires_at && Date.parse(k.expires_at) < now) return 'expired';
  return 'active';
}

const stateTone = { active: 'success', revoked: 'neutral', expired: 'danger' } as const;

function UsageCard() {
  const { t } = useI18n();
  const origin = useOrigin();
  const { data: settings } = useApi<Settings>('/settings', undefined, { scoped: false, revalidateOnFocus: false });
  const base = settings?.urls.api_base_url || (origin ? `${origin}/v1` : '/v1');
  const example = `curl -H "Authorization: Bearer dfk_live_…" \\\n  -H "Accept: application/vnd.api+json" \\\n  ${base}/stores`;
  return (
    <Card>
      <CardHeader title={t('team.apiKeys.usageTitle')} description={t('team.apiKeys.usageDesc')} />
      <CardBody className="space-y-4">
        <div className="space-y-1.5">
          <p className="text-xs font-medium">{t('team.apiKeys.baseUrl')}</p>
          <CopyField value={base} label={t('team.apiKeys.baseUrl')} />
        </div>
        <div className="space-y-1.5">
          <p className="text-xs font-medium">{t('team.apiKeys.example')}</p>
          <div className="relative rounded-lg border border-line bg-surface-2">
            <div className="absolute right-1.5 top-1.5">
              <CopyButton value={example} label={t('team.apiKeys.example')} />
            </div>
            <pre className="overflow-x-auto whitespace-pre p-3 pr-10 text-xs leading-relaxed" tabIndex={0}>
              {example}
            </pre>
          </div>
        </div>
      </CardBody>
    </Card>
  );
}

function KeysCard() {
  const { t } = useI18n();
  const can = useCan();
  const confirm = useConfirm();
  const { run } = useAction();
  const list = useList<ApiKey>('/api-keys', {});
  const [now] = useState(() => Date.now());
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<ApiKeyCreated | null>(null);

  async function revoke(k: ApiKey) {
    const ok = await confirm({
      title: t('team.apiKeys.revokeTitle', { name: k.name }),
      description: t('team.apiKeys.revokeBody'),
      confirmLabel: t('team.apiKeys.revoke'),
      tone: 'danger',
    });
    if (!ok) return;
    const res = await run(() => adminApi.delete(`/api-keys/${k.id}`), { success: t('team.apiKeys.revoked') });
    if (res.ok) void list.mutate();
  }

  const columns: Column<ApiKey>[] = [
    {
      key: 'name',
      header: t('common.name'),
      cell: (k) => (
        <div className="min-w-0">
          <p className="truncate font-medium">{k.name}</p>
          <p className="truncate text-xs text-subtle sm:hidden">
            <code>{k.prefix}…</code>
          </p>
        </div>
      ),
    },
    { key: 'prefix', header: t('team.apiKeys.col.prefix'), hideBelow: 'sm', cell: (k) => <code className="rounded bg-surface-3 px-1.5 py-0.5 text-xs">{k.prefix}…</code> },
    {
      key: 'mode',
      header: t('team.apiKeys.col.mode'),
      hideBelow: 'md',
      cell: (k) => <Badge tone={k.test_mode ? 'warning' : 'accent'}>{k.test_mode ? t('team.apiKeys.modeTest') : t('team.apiKeys.modeLive')}</Badge>,
    },
    { key: 'owner', header: t('team.apiKeys.col.owner'), hideBelow: 'xl', cell: (k) => <span className="text-muted">{k.owner_email ?? '—'}</span> },
    {
      key: 'last_used',
      header: t('team.apiKeys.col.lastUsed'),
      hideBelow: 'lg',
      cell: (k) => (k.last_used_at ? <DateText value={k.last_used_at} relative /> : <span className="text-subtle">{t('team.apiKeys.neverUsed')}</span>),
    },
    {
      key: 'expires',
      header: t('team.apiKeys.col.expires'),
      hideBelow: 'lg',
      cell: (k) => (k.expires_at ? <DateText value={k.expires_at} style="date" /> : <span className="text-subtle">{t('common.never')}</span>),
    },
    {
      key: 'status',
      header: t('common.status'),
      cell: (k) => {
        const s = keyState(k, now);
        return (
          <Badge dot tone={stateTone[s]}>
            {t(`team.apiKeys.status.${s}`)}
          </Badge>
        );
      },
    },
    { key: 'created', header: t('common.created'), hideBelow: 'xl', cell: (k) => <DateText value={k.created_at} style="date" className="text-muted" /> },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      align: 'right',
      cell: (k) => (
        <ActionMenu
          items={[{ key: 'revoke', label: t('team.apiKeys.revoke'), icon: <Trash2 />, tone: 'danger', hidden: !can.owner || !!k.revoked_at, onSelect: () => void revoke(k) }]}
        />
      ),
    },
  ];

  return (
    <>
      <Card>
        <CardHeader
          title={t('team.apiKeys.listTitle')}
          description={t('team.apiKeys.description')}
          actions={
            can.owner && (
              <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
                {t('team.apiKeys.create')}
              </Button>
            )
          }
        />
        {!can.owner && <Callout tone="info" className="m-4 mb-0">{t('team.apiKeys.ownerOnly')}</Callout>}
        <Toolbar className="border-b-0 pb-0">
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('team.apiKeys.searchPlaceholder')} />
        </Toolbar>
        <DataTable
          className="mt-3 border-t border-line"
          columns={columns}
          rows={list.rows}
          rowKey={(k) => k.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          empty={{
            icon: <KeyRound className="size-5" />,
            title: t('team.apiKeys.empty'),
            description: t('team.apiKeys.emptyDesc'),
            action: can.owner ? (
              <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
                {t('team.apiKeys.create')}
              </Button>
            ) : undefined,
          }}
        />
      </Card>
      {creating && (
        <CreateKeyDialog
          onClose={() => setCreating(false)}
          onCreated={(key) => {
            setCreating(false);
            setCreated(key);
            void list.mutate();
          }}
        />
      )}
      {created && <TokenDialog apiKey={created} onClose={() => setCreated(null)} />}
    </>
  );
}

export default function ApiKeysPage() {
  const { t } = useI18n();
  const can = useCan();
  usePageTitle(t('team.apiKeys.title'));

  if (!can.admin) {
    return (
      <Card>
        <EmptyState icon={<Lock className="size-5" />} title={t('team.apiKeys.forbidden')} description={t('team.apiKeys.forbiddenDesc')} />
      </Card>
    );
  }
  return (
    <div className="space-y-5">
      <UsageCard />
      <KeysCard />
    </div>
  );
}
