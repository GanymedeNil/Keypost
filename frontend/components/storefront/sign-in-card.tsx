'use client';

import { useState, type FormEvent } from 'react';
import { MailCheck, Mail } from 'lucide-react';
import { storefrontApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCooldown } from '@/lib/storefront';
import { Button, Card, CardBody, Field, Input, useToast } from '@/components/kit';

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Email → magic link → "check your inbox". The API always answers 204 (never reveals whether the email exists). */
export function SignInCard() {
  const { t } = useI18n();
  const toast = useToast();
  const [email, setEmail] = useState('');
  const [touched, setTouched] = useState(false);
  const [sentTo, setSentTo] = useState<string | null>(null);
  const { run, pending, fieldErrors } = useAction();
  const { remaining, start } = useCooldown(30);

  const trimmed = email.trim();
  const invalid = touched && !EMAIL_RE.test(trimmed);

  async function send(address: string, isResend: boolean) {
    const res = await run(() => storefrontApi.post('/auth/magic-link', { email: address }));
    if (!res.ok) return;
    setSentTo(address);
    start();
    if (isResend) toast.success(t('storefront.sent.resent'));
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    setTouched(true);
    if (!EMAIL_RE.test(trimmed)) return;
    void send(trimmed, false);
  }

  if (sentTo) {
    return (
      <Card className="mx-auto max-w-md">
        <CardBody className="flex flex-col items-center gap-3 px-6 py-8 text-center sm:px-8">
          <span className="grid size-12 place-items-center rounded-full bg-success-soft text-success-text">
            <MailCheck className="size-6" aria-hidden />
          </span>
          <h1 className="text-lg font-semibold tracking-tight">{t('storefront.sent.title')}</h1>
          <p className="text-sm text-muted-foreground" role="status">
            {t('storefront.sent.desc', { email: sentTo })}
          </p>
          <p className="text-xs text-muted-foreground">{t('storefront.sent.spamHint')}</p>
          <div className="mt-2 flex w-full flex-col gap-2 sm:flex-row sm:justify-center">
            <Button
              size="lg"
              className="h-11"
              disabled={remaining > 0}
              loading={pending}
              onClick={() => void send(sentTo, true)}
            >
              {remaining > 0 ? t('storefront.sent.resendIn', { seconds: remaining }) : t('storefront.sent.resend')}
            </Button>
            <Button
              size="lg"
              variant="ghost"
              className="h-11"
              onClick={() => {
                setSentTo(null);
                setTouched(false);
              }}
            >
              {t('storefront.sent.differentEmail')}
            </Button>
          </div>
        </CardBody>
      </Card>
    );
  }

  return (
    <Card className="mx-auto max-w-md">
      <CardBody className="px-6 py-8 sm:px-8">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <span className="grid size-12 place-items-center rounded-full bg-accent text-primary">
            <Mail className="size-6" aria-hidden />
          </span>
          <h1 className="text-lg font-semibold tracking-tight">{t('storefront.signIn.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('storefront.signIn.desc')}</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4" noValidate>
          <Field label={t('storefront.signIn.email')} error={invalid ? t('storefront.signIn.invalidEmail') : fieldErrors.email}>
            <Input
              type="email"
              inputMode="email"
              autoComplete="email"
              autoFocus
              required
              className="h-11 text-base"
              placeholder={t('storefront.signIn.emailPlaceholder')}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={() => email && setTouched(true)}
            />
          </Field>
          <Button type="submit" variant="primary" size="lg" className="h-11 w-full" loading={pending}>
            {pending ? t('storefront.signIn.sending') : t('storefront.signIn.submit')}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
