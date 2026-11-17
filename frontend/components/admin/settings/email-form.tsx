'use client';

import { useState, type FormEvent } from 'react';
import { Send } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import type { Settings, SettingsUpdate } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { Button, Card, CardBody, CardHeader, Callout, Field, Input, Select } from '@/components/ui';
import { useAdmin } from '../admin-context';
import { SaveBar } from './save-bar';
import { emptySecret, SecretField, secretUpdate, type SecretDraft } from './secret-field';
import type { SettingsCtx } from './settings-gate';

interface Draft {
  host: string;
  port: string;
  user: string;
  password: SecretDraft;
  from: string;
  security: string;
}

function init(s: Settings): Draft {
  const m = s.smtp;
  return { host: m.host ?? '', port: String(m.port ?? ''), user: m.user ?? '', password: emptySecret, from: m.from ?? '', security: m.security || 'starttls' };
}

const validPort = (v: string) => /^\d+$/.test(v) && Number(v) >= 1 && Number(v) <= 65535;

function diff(s: Settings, d: Draft): NonNullable<SettingsUpdate['smtp']> {
  const m = s.smtp;
  const out: NonNullable<SettingsUpdate['smtp']> = {};
  if (d.host.trim() !== (m.host ?? '')) out.host = d.host.trim();
  if (validPort(d.port) && Number(d.port) !== m.port) out.port = Number(d.port);
  if (d.user !== (m.user ?? '')) out.user = d.user;
  const pw = secretUpdate(d.password);
  if (pw !== undefined) out.password = pw;
  if (d.from.trim() !== (m.from ?? '')) out.from = d.from.trim();
  if (d.security !== (m.security || 'starttls')) out.security = d.security;
  return out;
}

export function SmtpForm({ ctx }: { ctx: SettingsCtx }) {
  const { t } = useI18n();
  const { settings, readOnly, pending, fieldErrors, save } = ctx;
  const [draft, setDraft] = useState(() => init(settings));
  const update = diff(settings, draft);
  const portOk = validPort(draft.port);
  const dirty = Object.keys(update).length > 0;
  const patch = (p: Partial<Draft>) => setDraft((d) => ({ ...d, ...p }));
  const err = (key: string) => fieldErrors[`smtp.${key}`];

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!dirty || readOnly || !portOk) return;
    await save({ smtp: update });
  }

  return (
    <div className="space-y-6">
      <form onSubmit={submit} className="space-y-6" noValidate>
        <Card>
          <CardHeader title={t('settings.email.smtp.title')} description={t('settings.email.smtp.description')} />
          <CardBody className="space-y-5">
            <div className="grid gap-5 sm:grid-cols-[1fr_8rem]">
              <Field label={t('settings.email.host')} error={err('host')}>
                <Input value={draft.host} onChange={(e) => patch({ host: e.target.value })} disabled={readOnly} placeholder="smtp.example.com" autoComplete="off" />
              </Field>
              <Field label={t('settings.email.port')} error={err('port') ?? (portOk ? undefined : t('settings.email.invalidPort'))}>
                <Input
                  inputMode="numeric"
                  value={draft.port}
                  onChange={(e) => patch({ port: e.target.value.trim() })}
                  disabled={readOnly}
                  className="tabular-nums"
                />
              </Field>
            </div>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label={t('settings.email.user')} error={err('user')}>
                <Input value={draft.user} onChange={(e) => patch({ user: e.target.value })} disabled={readOnly} autoComplete="off" />
              </Field>
              <SecretField
                label={t('settings.email.password')}
                current={settings.smtp.password}
                draft={draft.password}
                onChange={(password) => patch({ password })}
                disabled={readOnly}
                error={err('password')}
              />
            </div>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label={t('settings.email.from')} error={err('from')}>
                <Input value={draft.from} onChange={(e) => patch({ from: e.target.value })} disabled={readOnly} placeholder={t('settings.email.fromPlaceholder')} />
              </Field>
              <Field label={t('settings.email.security')} error={err('security')}>
                <Select value={draft.security} onChange={(e) => patch({ security: e.target.value })} disabled={readOnly}>
                  <option value="none">{t('settings.email.security.none')}</option>
                  <option value="starttls">{t('settings.email.security.starttls')}</option>
                  <option value="tls">{t('settings.email.security.tls')}</option>
                </Select>
              </Field>
            </div>
          </CardBody>
        </Card>
        {!readOnly && <SaveBar dirty={dirty} pending={pending} disabled={!portOk} onReset={() => setDraft(init(settings))} />}
      </form>
      <TestEmailCard unsaved={dirty} />
    </div>
  );
}

function TestEmailCard({ unsaved }: { unsaved: boolean }) {
  const { t } = useI18n();
  const { user, can } = useAdmin();
  const [to, setTo] = useState(user.email);
  const { run, pending, fieldErrors } = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const recipient = to.trim();
    if (!recipient) return;
    await run(() => adminApi.post('/settings/smtp/test', { to: recipient }), { success: t('settings.email.test.sent', { to: recipient }) });
  }

  return (
    <Card>
      <CardHeader title={t('settings.email.test.title')} description={t('settings.email.test.description')} />
      <CardBody>
        <form onSubmit={submit} className="flex flex-wrap items-end gap-3" noValidate>
          <Field label={t('settings.email.test.to')} error={fieldErrors.to} className="min-w-64 flex-1">
            <Input type="email" value={to} onChange={(e) => setTo(e.target.value)} required />
          </Field>
          <Button type="submit" variant="primary" icon={<Send className="size-3.5" />} loading={pending} disabled={!can.write || !to.trim()}>
            {t('settings.email.test.send')}
          </Button>
        </form>
        {unsaved && <Callout tone="warning" className="mt-4">{t('settings.email.test.saveFirst')}</Callout>}
      </CardBody>
    </Card>
  );
}
