'use client';

import { Suspense, useEffect, useRef, useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import useSWR, { mutate as globalMutate } from 'swr';
import QRCode from 'qrcode';
import { BrandLogo } from '@/components/brand-mark';
import { adminApi, ApiError, fetcher } from '@/lib/api';
import type { User } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, Callout, CopyButton, Field, Input, Skeleton, usePageTitle } from '@/components/ui';
import { LanguageMenu, ThemeMenu } from '@/components/admin/topbar';
import { RecoveryCodes } from '@/components/admin/mfa/recovery-codes';

function safeNext(raw: string | null): string {
  if (raw && raw.startsWith('/admin') && !raw.startsWith('//') && !raw.startsWith('/admin/login') && !raw.startsWith('/admin/mfa')) return raw;
  return '/admin';
}

interface Setup {
  secret: string;
  otpauth_uri: string;
}

function groupSecret(s: string) {
  return s.replace(/(.{4})/g, '$1 ').trim();
}

function SetupStep({ onEnabled }: { onEnabled: (codes: string[]) => void }) {
  const { t } = useI18n();
  const [setup, setSetup] = useState<Setup | null>(null);
  const [qr, setQr] = useState<string | null>(null);
  const [code, setCode] = useState('');
  const started = useRef(false);
  const start = useAction();
  const enable = useAction();

  useEffect(() => {
    // one call only: every call replaces the pending secret, and the QR code must match the one entered
    if (started.current) return;
    started.current = true;
    void start.run(() => adminApi.post<Setup>('/auth/mfa/setup')).then((res) => res.ok && setSetup(res.data));
  }, [start]);

  useEffect(() => {
    if (!setup) return;
    let live = true;
    QRCode.toDataURL(setup.otpauth_uri, { margin: 1, width: 208, errorCorrectionLevel: 'M' }).then((url) => live && setQr(url));
    return () => {
      live = false;
    };
  }, [setup]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await enable.run(() => adminApi.post<{ recovery_codes: string[] }>('/auth/mfa/enable', { code }), { silent: true });
    if (res.ok) onEnabled(res.data.recovery_codes);
  }

  return (
    <form onSubmit={submit} className="space-y-5" noValidate>
      <p className="text-sm text-muted">{t('mfa.setup.desc')}</p>
      <div className="flex flex-col items-center gap-3">
        <p className="text-xs font-medium text-subtle">{t('mfa.setup.scan')}</p>
        <div className="grid size-[224px] place-items-center rounded-xl border border-line bg-white p-2">
          {qr ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={qr} width={208} height={208} alt="QR code" />
          ) : (
            <Skeleton className="size-[208px]" />
          )}
        </div>
        {setup && (
          <div className="w-full text-center">
            <p className="text-xs text-subtle">{t('mfa.setup.manual')}</p>
            <p className="mt-1 flex items-center justify-center gap-1 font-mono text-sm tracking-wider">
              <span>{groupSecret(setup.secret)}</span>
              <CopyButton value={setup.secret} label="secret key" />
            </p>
          </div>
        )}
      </div>
      <Field label={t('mfa.setup.code')} error={enable.fieldErrors.code ? t('mfa.error.enable') : undefined}>
        <Input
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9 ]*"
          maxLength={7}
          value={code}
          onChange={(e) => setCode(e.target.value)}
          className="text-center font-mono text-lg tracking-[0.3em]"
          disabled={!setup}
          autoFocus
        />
      </Field>
      <Button type="submit" variant="primary" size="lg" className="w-full" loading={enable.pending} disabled={!setup || code.replace(/\s/g, '').length < 6}>
        {setup ? t('mfa.setup.enable') : t('mfa.setup.preparing')}
      </Button>
    </form>
  );
}

function RecoveryStep({ codes, onDone }: { codes: string[]; onDone: () => void }) {
  const { t } = useI18n();
  const [saved, setSaved] = useState(false);
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted">{t('mfa.recovery.desc')}</p>
      <RecoveryCodes codes={codes} />
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
        {t('mfa.recovery.saved')}
      </label>
      <Button variant="primary" size="lg" className="w-full" disabled={!saved} onClick={onDone}>
        {t('mfa.recovery.continue')}
      </Button>
    </div>
  );
}

function VerifyStep({ onVerified }: { onVerified: () => void }) {
  const { t } = useI18n();
  const [recovery, setRecovery] = useState(false);
  const [value, setValue] = useState('');
  const [locked, setLocked] = useState(false);
  const { run, pending, fieldErrors } = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setLocked(false);
    const body = recovery ? { recovery_code: value } : { code: value };
    const res = await run(() => adminApi.post<User>('/auth/mfa/verify', body), { silent: true });
    if (res.ok) return onVerified();
    setLocked(res.error instanceof ApiError && res.error.status === 429);
    setValue('');
  }

  return (
    <form onSubmit={submit} className="space-y-5" noValidate>
      <p className="text-sm text-muted">{recovery ? t('mfa.verify.recoveryHint') : t('mfa.verify.desc')}</p>
      {locked && <Callout tone="danger">{t('mfa.error.locked')}</Callout>}
      <Field
        label={recovery ? t('mfa.verify.recoveryLabel') : t('mfa.verify.code')}
        error={fieldErrors.code || fieldErrors.recovery_code ? t(recovery ? 'mfa.error.recovery' : 'mfa.error.code') : undefined}
      >
        <Input
          key={recovery ? 'recovery' : 'code'}
          inputMode={recovery ? 'text' : 'numeric'}
          autoComplete="one-time-code"
          autoCapitalize="none"
          spellCheck={false}
          maxLength={recovery ? 12 : 7}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="text-center font-mono text-lg tracking-[0.25em]"
          autoFocus
        />
      </Field>
      <Button type="submit" variant="primary" size="lg" className="w-full" loading={pending} disabled={value.replace(/[\s-]/g, '').length < 6}>
        {t('mfa.verify.submit')}
      </Button>
      <button
        type="button"
        className="mx-auto block text-sm text-accent-text hover:underline"
        onClick={() => {
          setRecovery((r) => !r);
          setValue('');
        }}
      >
        {recovery ? t('mfa.verify.useCode') : t('mfa.verify.useRecovery')}
      </button>
    </form>
  );
}

function MfaInner() {
  const { t } = useI18n();
  const router = useRouter();
  const next = safeNext(useSearchParams().get('next'));
  const { data: user, error, mutate } = useSWR<User>(adminApi.url('/me'), fetcher, { revalidateOnFocus: false });
  const [codes, setCodes] = useState<string[] | null>(null);
  const enrolled = user?.mfa?.enabled ?? false;
  const title = codes ? t('mfa.recovery.title') : enrolled ? t('mfa.verify.title') : t('mfa.setup.title');
  usePageTitle(title);

  // already through (or MFA is off): nothing to do here
  useEffect(() => {
    if (user && !codes && (!user.mfa?.required || user.mfa.verified)) router.replace(next);
  }, [user, codes, next, router]);

  async function finish() {
    await mutate();
    router.replace(next);
  }

  async function signOut() {
    try {
      await adminApi.post('/auth/logout', undefined, { redirectOn401: false });
    } catch {
      /* already signed out */
    }
    await globalMutate(() => true, undefined, { revalidate: false });
    router.replace('/admin/login');
  }

  return (
    <div className="relative grid min-h-dvh place-items-center px-4 py-10">
      <div className="absolute right-3 top-3 flex items-center gap-1">
        <LanguageMenu />
        <ThemeMenu />
      </div>
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center text-center">
          <BrandLogo className="mb-5 h-11 w-auto text-fg" />
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          {user && <p className="mt-1 text-sm text-muted">{user.email}</p>}
        </div>
        <div className="rounded-xl border border-line bg-surface p-5 shadow-card sm:p-6">
          {error ? (
            <Callout tone="danger">{error instanceof Error ? error.message : t('errors.generic')}</Callout>
          ) : !user ? (
            <div className="space-y-4" role="status" aria-busy="true">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : codes ? (
            <RecoveryStep codes={codes} onDone={finish} />
          ) : enrolled ? (
            <VerifyStep onVerified={finish} />
          ) : (
            <SetupStep onEnabled={setCodes} />
          )}
        </div>
        <button type="button" onClick={signOut} className="mx-auto mt-4 block text-sm text-muted hover:text-fg hover:underline">
          {t('mfa.signOut')}
        </button>
      </div>
    </div>
  );
}

export default function MfaPage() {
  return (
    <Suspense>
      <MfaInner />
    </Suspense>
  );
}
