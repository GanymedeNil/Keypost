'use client';

import { useState, type FormEvent } from 'react';
import { KeyRound, ShieldCheck } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Badge, Button, Card, CardBody, CardHeader, Field, Input, Modal } from '@/components/ui';
import { useAdmin } from '../admin-context';
import { RecoveryCodes } from '../mfa/recovery-codes';

/** Profile card: two-factor authentication is mandatory, so it can be inspected and refreshed here but never turned off. */
export function MfaCard() {
  const { t } = useI18n();
  const { user, refreshUser } = useAdmin();
  const [open, setOpen] = useState(false);
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[] | null>(null);
  const { run, pending, fieldErrors } = useAction();

  function close() {
    setOpen(false);
    setCode('');
    setCodes(null);
    void refreshUser();
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await run(() => adminApi.post<{ recovery_codes: string[] }>('/me/mfa/recovery-codes', { code }), { silent: true });
    if (res.ok) setCodes(res.data.recovery_codes);
  }

  return (
    <>
      <Card>
        <CardHeader
          title={t('mfa.card.title')}
          description={t('mfa.card.description')}
          actions={
            <Badge tone="success">
              <ShieldCheck className="size-3" />
              {t('mfa.card.enabled')}
            </Badge>
          }
        />
        <CardBody className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted">{t('mfa.card.codesLeft', { count: user.mfa?.recovery_codes_left ?? 0 })}</p>
            <Button icon={<KeyRound className="size-3.5" />} onClick={() => setOpen(true)}>
              {t('mfa.card.regenerate')}
            </Button>
          </div>
          <p className="text-xs text-subtle">{t('mfa.card.lost')}</p>
        </CardBody>
      </Card>
      <Modal
        open={open}
        onOpenChange={(o) => !o && close()}
        title={codes ? t('mfa.card.newCodesTitle') : t('mfa.card.regenerateTitle')}
        description={codes ? t('mfa.recovery.desc') : t('mfa.card.regenerateDesc')}
        size="sm"
        footer={
          codes ? (
            <Button variant="primary" onClick={close}>
              {t('mfa.card.done')}
            </Button>
          ) : (
            <>
              <Button onClick={close}>{t('common.cancel')}</Button>
              <Button type="submit" form="mfa-regenerate" variant="primary" loading={pending} disabled={code.replace(/\s/g, '').length < 6}>
                {t('mfa.card.generate')}
              </Button>
            </>
          )
        }
      >
        {codes ? (
          <RecoveryCodes codes={codes} />
        ) : (
          <form id="mfa-regenerate" onSubmit={submit} noValidate>
            <Field label={t('mfa.verify.code')} error={fieldErrors.code ? t('mfa.error.code') : undefined}>
              <Input
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={7}
                value={code}
                onChange={(e) => setCode(e.target.value)}
                className="text-center font-mono text-lg tracking-[0.3em]"
                autoFocus
              />
            </Field>
          </form>
        )}
      </Modal>
    </>
  );
}
