'use client';

import { useState } from 'react';
import { Ban, CheckCircle2, Lock, Pencil, Plus, ShieldOff, Trash2, Users } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useList } from '@/lib/use-api';
import type { Role, User } from '@/lib/types';
import {
  ActionMenu,
  Badge,
  Button,
  Card,
  CardHeader,
  DataTable,
  DateText,
  EmptyState,
  FilterSelect,
  SearchInput,
  StatusBadge,
  Toolbar,
  useConfirm,
  useConfirmDelete,
  usePageTitle,
  type Column,
  type Tone,
} from '@/components/ui';
import { useAdmin } from '@/components/admin/admin-context';
import { UserDialog } from '@/components/admin/team/user-dialog';

const roleTone: Record<Role, Tone> = { owner: 'accent', admin: 'info', viewer: 'neutral' };
const ROLES: Role[] = ['owner', 'admin', 'viewer'];

function TeamContent() {
  const { t, te } = useI18n();
  const { user: me } = useAdmin();
  const confirm = useConfirm();
  const confirmDelete = useConfirmDelete();
  const { run } = useAction();
  const list = useList<User>('/users', { filters: ['role'] });
  const [dialog, setDialog] = useState<{ user?: User } | null>(null);

  async function setDisabled(u: User, disabled: boolean) {
    const res = await run(() => adminApi.patch<User>(`/users/${u.id}`, { disabled }), {
      success: disabled ? t('team.users.disabled') : t('team.users.enabled'),
    });
    if (res.ok) void list.mutate();
  }

  async function resetMfa(u: User) {
    const ok = await confirm({
      title: t('team.users.mfaResetTitle'),
      description: t('team.users.mfaResetBody', { name: u.name || u.email }),
      confirmLabel: t('team.users.mfaReset'),
    });
    if (!ok) return;
    const res = await run(() => adminApi.post(`/users/${u.id}/mfa/reset`), { success: t('team.users.mfaResetDone') });
    if (res.ok) void list.mutate();
  }

  async function remove(u: User) {
    if (!(await confirmDelete(u.name || u.email, t('team.users.deleteBody')))) return;
    const res = await run(() => adminApi.delete(`/users/${u.id}`), { success: t('team.users.deleted') });
    if (res.ok) void list.mutate();
  }

  const columns: Column<User>[] = [
    {
      key: 'member',
      header: t('team.users.col.member'),
      cell: (u) => (
        <div className="min-w-0">
          <p className="flex items-center gap-2 font-medium">
            <span className="truncate">{u.name || u.email}</span>
            {u.id === me.id && <Badge tone="accent">{t('team.users.you')}</Badge>}
          </p>
          <p className="truncate text-xs text-subtle">{u.email}</p>
          <div className="mt-1.5 flex flex-wrap gap-1 sm:hidden">
            <Badge tone={roleTone[u.role]}>{te(`role.${u.role}`, u.role)}</Badge>
            <StatusBadge status={u.disabled ? 'disabled' : 'active'} />
          </div>
        </div>
      ),
    },
    { key: 'role', header: t('team.users.role'), hideBelow: 'sm', cell: (u) => <Badge tone={roleTone[u.role]}>{te(`role.${u.role}`, u.role)}</Badge> },
    {
      key: 'status',
      header: t('common.status'),
      hideBelow: 'sm',
      cell: (u) => <StatusBadge status={u.disabled ? 'disabled' : 'active'} />,
    },
    {
      key: 'signin',
      header: t('team.users.col.signIn'),
      hideBelow: 'md',
      cell: (u) => (
        <div className="flex flex-wrap gap-1">
          {u.has_password && <Badge>{t('team.users.methodPassword')}</Badge>}
          {u.stripe_account_id && (
            <Badge tone="info" className="max-w-40">
              <span className="truncate">
                {t('team.users.methodStripe')}: {u.stripe_account_label || u.stripe_account_id}
              </span>
            </Badge>
          )}
          {!u.has_password && !u.stripe_account_id && <span className="text-subtle">—</span>}
        </div>
      ),
    },
    {
      key: 'mfa',
      header: t('team.users.col.mfa'),
      hideBelow: 'md',
      cell: (u) => (u.mfa_enabled ? <Badge tone="success">{t('team.users.mfaOn')}</Badge> : <Badge tone="warning">{t('team.users.mfaOff')}</Badge>),
    },
    {
      key: 'last_login',
      header: t('team.users.col.lastLogin'),
      hideBelow: 'lg',
      cell: (u) => (u.last_login_at ? <DateText value={u.last_login_at} relative /> : <span className="text-subtle">{t('common.never')}</span>),
    },
    { key: 'created', header: t('common.created'), hideBelow: 'xl', cell: (u) => <DateText value={u.created_at} style="date" className="text-muted" /> },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      align: 'right',
      cell: (u) => (
        <ActionMenu
          items={[
            { key: 'edit', label: t('common.edit'), icon: <Pencil />, onSelect: () => setDialog({ user: u }) },
            {
              key: 'toggle',
              label: u.disabled ? t('team.users.enable') : t('team.users.disable'),
              icon: u.disabled ? <CheckCircle2 /> : <Ban />,
              onSelect: () => void setDisabled(u, !u.disabled),
            },
            ...(u.id !== me.id && u.mfa_enabled
              ? [{ key: 'mfa-reset', label: t('team.users.mfaReset'), icon: <ShieldOff />, onSelect: () => void resetMfa(u) }]
              : []),
            { key: 'delete', label: t('common.delete'), icon: <Trash2 />, tone: 'danger', separatorBefore: true, onSelect: () => void remove(u) },
          ]}
        />
      ),
    },
  ];

  return (
    <>
      <Card>
        <CardHeader
          title={t('team.users.title')}
          description={t('team.users.description')}
          actions={
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setDialog({})}>
              {t('team.users.add')}
            </Button>
          }
        />
        <Toolbar>
          <SearchInput value={list.q} onChange={list.setSearch} placeholder={t('team.users.searchPlaceholder')} />
          <FilterSelect
            aria-label={t('team.users.filterRole')}
            value={list.values.role}
            onChange={(v) => list.setFilter('role', v)}
            allLabel={t('team.users.role')}
            options={ROLES.map((r) => ({ value: r, label: te(`role.${r}`, r) }))}
          />
        </Toolbar>
        <DataTable
          columns={columns}
          rows={list.rows}
          rowKey={(u) => u.id}
          loading={list.isLoading}
          error={list.error}
          onRetry={() => void list.mutate()}
          meta={list.meta}
          onPageChange={list.setPage}
          filtered={list.hasFilters}
          empty={{ icon: <Users className="size-5" />, title: t('team.users.empty') }}
        />
      </Card>
      {dialog && (
        <UserDialog
          user={dialog.user}
          onClose={() => setDialog(null)}
          onSaved={() => {
            setDialog(null);
            void list.mutate();
          }}
        />
      )}
    </>
  );
}

export default function TeamPage() {
  const { t } = useI18n();
  const { can } = useAdmin();
  usePageTitle(t('team.users.title'));
  if (!can.owner) {
    return (
      <Card>
        <EmptyState icon={<Lock className="size-5" />} title={t('team.users.ownerOnly')} description={t('team.users.ownerOnlyDesc')} />
      </Card>
    );
  }
  return <TeamContent />;
}
