'use client';

import { Suspense, useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import useSWR, { mutate } from 'swr';
import { BrandLogo } from '@/components/brand-mark';
import { adminApi, ApiError, ADMIN_BASE } from '@/lib/api';
import type { AuthConfig, User } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useToast } from '@/components/ui';
import { errorText } from '@/lib/error-text';
import { Button, ButtonAnchor, Callout, Field, Input, Skeleton, usePageTitle } from '@/components/ui';
import { LanguageMenu, ThemeMenu } from '@/components/admin/topbar';

const ERROR_CODES = ['stripe_denied', 'stripe_state', 'stripe_not_allowed', 'stripe_exchange', 'disabled'];

function safeNext(raw: string | null): string {
  if (raw && raw.startsWith('/admin') && !raw.startsWith('//') && !raw.startsWith('/admin/login')) return raw;
  return '/admin';
}

function PasswordForm({ onDone }: { onDone: () => void }) {
  const { t } = useI18n();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { run, pending, fieldErrors } = useAction();
  const toast = useToast();
  const [invalid, setInvalid] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setInvalid(false);
    const res = await run(() => adminApi.post<{ user: User }>('/auth/login', { email, password }), { silent: true });
    if (res.ok) {
      await mutate(adminApi.url('/me'), res.data.user, { revalidate: false });
      onDone();
    } else if (res.error instanceof ApiError && res.error.status >= 400 && res.error.status < 500) {
      setInvalid(true);
    } else {
      toast.error(errorText(res.error, t));
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4" noValidate>
      {invalid && (
        <Callout tone="danger">{t('auth.invalidCredentials')}</Callout>
      )}
      <Field label={t('auth.email')} error={fieldErrors.email}>
        <Input type="email" autoComplete="username" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Field label={t('auth.password')} error={fieldErrors.password}>
        <Input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      <Button type="submit" variant="primary" size="lg" className="w-full" loading={pending}>
        {pending ? t('auth.signingIn') : t('auth.signIn')}
      </Button>
    </form>
  );
}

function SetupForm({ onDone }: { onDone: () => void }) {
  const { t } = useI18n();
  const [form, setForm] = useState({ setup_token: '', name: '', email: '', password: '', confirm: '' });
  const [mismatch, setMismatch] = useState(false);
  const [tokenInvalid, setTokenInvalid] = useState(false);
  const toast = useToast();
  const { run, pending, fieldErrors } = useAction();
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (form.password !== form.confirm) return setMismatch(true);
    setMismatch(false);
    setTokenInvalid(false);
    const { confirm, ...body } = form;
    void confirm;
    const res = await run(() => adminApi.post<{ user: User }>('/auth/setup', body), { silent: true });
    if (res.ok) {
      await mutate(adminApi.url('/me'), res.data.user, { revalidate: false });
      onDone();
    } else if (res.error instanceof ApiError && Object.keys(res.error.fields).length === 0) {
      // Without field errors a 4xx on /auth/setup means the setup token was rejected.
      if (res.error.status >= 400 && res.error.status < 500) setTokenInvalid(true);
      else toast.error(errorText(res.error, t));
    } else if (!(res.error instanceof ApiError)) {
      toast.error(errorText(res.error, t));
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4" noValidate>
      <Field label={t('auth.setupToken')} hint={t('auth.setupTokenHint')} error={fieldErrors.setup_token ?? (tokenInvalid ? t('auth.invalidSetupToken') : undefined)} required>
        <Input value={form.setup_token} onChange={set('setup_token')} autoFocus autoComplete="off" className="font-mono" />
      </Field>
      <Field label={t('auth.name')} error={fieldErrors.name} required>
        <Input value={form.name} onChange={set('name')} autoComplete="name" />
      </Field>
      <Field label={t('auth.email')} error={fieldErrors.email} required>
        <Input type="email" value={form.email} onChange={set('email')} autoComplete="username" />
      </Field>
      <Field label={t('auth.newPassword')} hint={t('auth.passwordHint')} error={fieldErrors.password} required>
        <Input type="password" minLength={10} value={form.password} onChange={set('password')} autoComplete="new-password" />
      </Field>
      <Field label={t('auth.confirmPassword')} error={mismatch ? t('auth.passwordMismatch') : undefined} required>
        <Input type="password" value={form.confirm} onChange={set('confirm')} autoComplete="new-password" />
      </Field>
      <Button type="submit" variant="primary" size="lg" className="w-full" loading={pending}>
        {pending ? t('auth.creating') : t('auth.createOwner')}
      </Button>
    </form>
  );
}

function LoginInner() {
  const { t, te } = useI18n();
  const router = useRouter();
  const params = useSearchParams();
  const next = safeNext(params.get('next'));
  const errorCode = params.get('error');
  const { data: config, error } = useSWR<AuthConfig>(adminApi.url('/auth/config'), { revalidateOnFocus: false });
  const done = () => router.replace(next);

  const setup = config?.setup_required;
  usePageTitle(setup ? t('auth.setupTitle') : t('auth.title'));
  return (
    <div className="relative grid min-h-dvh place-items-center px-4 py-10">
      <div className="absolute right-3 top-3 flex items-center gap-1">
        <LanguageMenu />
        <ThemeMenu />
      </div>
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center text-center">
          <BrandLogo className="mb-5 h-11 w-auto text-fg" />
          <h1 className="text-xl font-semibold tracking-tight">{setup ? t('auth.setupTitle') : t('auth.title')}</h1>
          <p className="mt-1 text-sm text-muted">{setup ? t('auth.setupDesc') : t('auth.subtitle')}</p>
        </div>
        <div className="rounded-xl border border-line bg-surface p-5 shadow-card sm:p-6">
          {errorCode && (
            <Callout tone="danger" className="mb-4">
              {ERROR_CODES.includes(errorCode) ? te(`auth.error.${errorCode}`, t('auth.error.unknown')) : t('auth.error.unknown')}
            </Callout>
          )}
          {error ? (
            <Callout tone="danger">{error instanceof Error ? error.message : t('errors.generic')}</Callout>
          ) : !config ? (
            <div className="space-y-4" role="status" aria-busy="true">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : setup ? (
            <SetupForm onDone={done} />
          ) : (
            <div className="space-y-4">
              {config.password_login && <PasswordForm onDone={done} />}
              {config.password_login && config.stripe_login && (
                <div className="flex items-center gap-3 text-xs text-subtle">
                  <span className="h-px flex-1 bg-line" />
                  {t('auth.or')}
                  <span className="h-px flex-1 bg-line" />
                </div>
              )}
              {config.stripe_login && (
                <ButtonAnchor href={`${ADMIN_BASE}/auth/stripe/start?mode=login`} variant="secondary" size="lg" className="w-full">
                  {t('auth.signInWithStripe')}
                </ButtonAnchor>
              )}
              {!config.password_login && !config.stripe_login && <Callout tone="warning">{t('auth.noMethods')}</Callout>}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginInner />
    </Suspense>
  );
}
