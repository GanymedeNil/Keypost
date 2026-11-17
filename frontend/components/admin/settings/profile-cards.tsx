'use client';

import { useState, type FormEvent } from 'react';
import { Link2, Unlink } from 'lucide-react';
import { adminApi, isApiError } from '@/lib/api';
import { errorText } from '@/lib/error-text';
import { useI18n } from '@/lib/i18n/provider';
import type { AuthConfig } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { Badge, Button, ButtonAnchor, Callout, Card, CardBody, CardFooter, CardHeader, Field, Input, useConfirm, useToast } from '@/components/ui';
import { useAdmin } from '../admin-context';

export function ProfileCard() {
  const { t, te } = useI18n();
  const { user, refreshUser } = useAdmin();
  const [name, setName] = useState(user.name);
  const { run, pending, fieldErrors } = useAction();
  const dirty = name.trim() !== user.name && name.trim() !== '';

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!dirty) return;
    const res = await run(() => adminApi.patch('/me', { name: name.trim() }), { success: t('toast.saved') });
    if (res.ok) await refreshUser();
  }

  return (
    <Card>
      <form onSubmit={submit} noValidate>
        <CardHeader title={t('settings.profile.title')} description={t('settings.profile.description')} />
        <CardBody className="grid gap-5 sm:grid-cols-2">
          <Field label={t('settings.profile.email')}>
            <Input value={user.email} readOnly />
          </Field>
          <Field label={t('settings.profile.role')}>
            <div className="flex h-8 items-center">
              <Badge tone={user.role === 'owner' ? 'accent' : 'neutral'}>{te(`role.${user.role}`, user.role)}</Badge>
            </div>
          </Field>
          <Field label={t('settings.profile.name')} error={fieldErrors.name} className="sm:col-span-2">
            <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" />
          </Field>
        </CardBody>
        <CardFooter>
          <Button type="submit" variant="primary" loading={pending} disabled={!dirty}>
            {t('common.save')}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

export function PasswordCard() {
  const { t } = useI18n();
  const { user, refreshUser } = useAdmin();
  const [form, setForm] = useState({ current: '', next: '', confirm: '' });
  const [mismatch, setMismatch] = useState(false);
  const { run, pending, fieldErrors } = useAction();
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setForm((f) => ({ ...f, [k]: value }));
  };

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (form.next !== form.confirm) return setMismatch(true);
    setMismatch(false);
    const res = await run(() => adminApi.post('/me/password', { current_password: form.current, new_password: form.next }), {
      success: t('settings.profile.password.changed'),
    });
    if (res.ok) {
      setForm({ current: '', next: '', confirm: '' });
      await refreshUser(); // has_password may have flipped
    }
  }

  // Stripe-only accounts have no password yet: the API accepts the first one without a current password.
  const firstPassword = !user.has_password;

  return (
    <Card>
      <form onSubmit={submit} noValidate>
        <CardHeader title={t('settings.profile.password.title')} description={t('settings.profile.password.description')} />
        <CardBody className="grid gap-5 sm:grid-cols-2">
          {firstPassword ? (
            <div className="sm:col-span-2">
              <Callout tone="info">{t('settings.profile.password.disabled')}</Callout>
            </div>
          ) : (
            <Field label={t('settings.profile.password.current')} error={fieldErrors.current_password} className="sm:col-span-2">
              <Input type="password" autoComplete="current-password" value={form.current} onChange={set('current')} />
            </Field>
          )}
          <Field label={t('settings.profile.password.new')} error={fieldErrors.new_password}>
            <Input type="password" autoComplete="new-password" minLength={10} value={form.next} onChange={set('next')} />
          </Field>
          <Field label={t('settings.profile.password.confirm')} error={mismatch ? t('settings.profile.password.mismatch') : undefined}>
            <Input type="password" autoComplete="new-password" value={form.confirm} onChange={set('confirm')} />
          </Field>
        </CardBody>
        <CardFooter>
          <Button type="submit" variant="primary" loading={pending} disabled={(!firstPassword && !form.current) || form.next.length < 10 || !form.confirm}>
            {firstPassword ? t('settings.profile.password.set') : t('settings.profile.password.change')}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

export function StripeAccountCard() {
  const { t } = useI18n();
  const toast = useToast();
  const confirm = useConfirm();
  const { user, refreshUser } = useAdmin();
  const { data: config } = useApi<AuthConfig>('/auth/config', undefined, { scoped: false, revalidateOnFocus: false });
  const { run, pending } = useAction();

  async function unlink() {
    const ok = await confirm({
      title: t('settings.profile.stripe.unlinkTitle'),
      description: t('settings.profile.stripe.unlinkBody'),
      confirmLabel: t('settings.profile.stripe.unlink'),
    });
    if (!ok) return;
    const res = await run(() => adminApi.delete('/me/stripe'), { silent: true, success: t('settings.profile.stripe.unlinked') });
    if (res.ok) await refreshUser();
    else toast.error(isApiError(res.error) && res.error.status === 409 ? t('settings.profile.stripe.needsPassword') : errorText(res.error, t));
  }

  const linked = !!user.stripe_account_id;
  return (
    <Card>
      <CardHeader title={t('settings.profile.stripe.title')} description={t('settings.profile.stripe.description')} />
      <CardBody className="space-y-3">
        {linked ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs text-subtle">{t('settings.profile.stripe.linked')}</p>
              {user.stripe_account_label && <p className="truncate text-sm font-medium">{user.stripe_account_label}</p>}
              <code className="text-xs text-muted">{user.stripe_account_id}</code>
            </div>
            <Button variant="danger-outline" icon={<Unlink className="size-3.5" />} onClick={unlink} loading={pending} disabled={!user.has_password}>
              {t('settings.profile.stripe.unlink')}
            </Button>
          </div>
        ) : (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted">{config && !config.stripe_login ? t('settings.profile.stripe.loginDisabled') : t('settings.profile.stripe.notLinked')}</p>
            {config?.stripe_login && (
              <ButtonAnchor href={adminApi.url('/auth/stripe/start', { mode: 'link' })} variant="primary" icon={<Link2 className="size-3.5" />}>
                {t('settings.profile.stripe.link')}
              </ButtonAnchor>
            )}
          </div>
        )}
        {linked && !user.has_password && <p className="text-xs text-warning-text">{t('settings.profile.stripe.needsPassword')}</p>}
      </CardBody>
    </Card>
  );
}
