'use client';

import { useState, type FormEvent } from 'react';
import { Card, CardBody, CardHeader, Field, Input, SwitchField } from '@/components/kit';
import { useI18n } from '@/lib/i18n/provider';
import type { Settings, SettingsUpdate } from '@/lib/types';
import { humanDuration } from './duration';
import { SaveBar } from './save-bar';
import type { SettingsCtx } from './settings-gate';

interface Draft {
  disable_on_refund: boolean;
  limit_per_day: string;
  url_ttl_seconds: string;
  success_url: string;
  cancel_url: string;
}

function init(s: Settings): Draft {
  return {
    disable_on_refund: s.license.disable_on_refund,
    limit_per_day: String(s.download.limit_per_day ?? 0),
    url_ttl_seconds: String(s.download.url_ttl_seconds ?? 0),
    success_url: s.checkout.success_url ?? '',
    cancel_url: s.checkout.cancel_url ?? '',
  };
}

const isCount = (v: string) => /^\d+$/.test(v);

function diff(s: Settings, d: Draft): SettingsUpdate {
  const out: SettingsUpdate = {};
  if (d.disable_on_refund !== s.license.disable_on_refund) out.license = { disable_on_refund: d.disable_on_refund };
  const download: NonNullable<SettingsUpdate['download']> = {};
  if (isCount(d.limit_per_day) && Number(d.limit_per_day) !== s.download.limit_per_day) download.limit_per_day = Number(d.limit_per_day);
  if (isCount(d.url_ttl_seconds) && Number(d.url_ttl_seconds) !== s.download.url_ttl_seconds) download.url_ttl_seconds = Number(d.url_ttl_seconds);
  if (Object.keys(download).length) out.download = download;
  const checkout: NonNullable<SettingsUpdate['checkout']> = {};
  if (d.success_url.trim() !== (s.checkout.success_url ?? '')) checkout.success_url = d.success_url.trim();
  if (d.cancel_url.trim() !== (s.checkout.cancel_url ?? '')) checkout.cancel_url = d.cancel_url.trim();
  if (Object.keys(checkout).length) out.checkout = checkout;
  return out;
}

export function LicenseForm({ ctx }: { ctx: SettingsCtx }) {
  const { t, locale } = useI18n();
  const { settings, readOnly, pending, fieldErrors, save } = ctx;
  const [draft, setDraft] = useState(() => init(settings));
  const update = diff(settings, draft);
  const dirty = Object.keys(update).length > 0;
  const numbersOk = isCount(draft.limit_per_day) && isCount(draft.url_ttl_seconds);
  const patch = (p: Partial<Draft>) => setDraft((d) => ({ ...d, ...p }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!dirty || readOnly || !numbersOk) return;
    await save(update);
  }

  return (
    <form onSubmit={submit} className="space-y-6" noValidate>
      <Card>
        <CardHeader title={t('settings.license.title')} />
        <CardBody>
          <SwitchField
            label={t('settings.license.disableOnRefund')}
            description={t('settings.license.disableOnRefundDesc')}
            checked={draft.disable_on_refund}
            onCheckedChange={(disable_on_refund) => patch({ disable_on_refund })}
            disabled={readOnly}
          />
        </CardBody>
      </Card>

      <Card>
        <CardHeader title={t('settings.download.title')} />
        <CardBody className="grid gap-5 sm:grid-cols-2">
          <Field
            label={t('settings.download.limitPerDay')}
            hint={t('settings.download.limitPerDayHint')}
            error={fieldErrors['download.limit_per_day'] ?? (isCount(draft.limit_per_day) ? undefined : t('settings.invalidNumber'))}
          >
            <Input inputMode="numeric" value={draft.limit_per_day} onChange={(e) => patch({ limit_per_day: e.target.value.trim() })} disabled={readOnly} className="tabular-nums" />
          </Field>
          <Field
            label={t('settings.download.urlTtl')}
            hint={isCount(draft.url_ttl_seconds) ? t('settings.download.urlTtlHint', { human: humanDuration(Number(draft.url_ttl_seconds), locale) }) : undefined}
            error={fieldErrors['download.url_ttl_seconds'] ?? (isCount(draft.url_ttl_seconds) ? undefined : t('settings.invalidNumber'))}
          >
            <Input inputMode="numeric" value={draft.url_ttl_seconds} onChange={(e) => patch({ url_ttl_seconds: e.target.value.trim() })} disabled={readOnly} className="tabular-nums" />
          </Field>
        </CardBody>
      </Card>

      <Card>
        <CardHeader title={t('settings.checkout.title')} description={t('settings.checkout.description')} />
        <CardBody className="grid gap-5 sm:grid-cols-2">
          <Field label={t('settings.checkout.successUrl')} hint={t('settings.checkout.successUrlHint')} error={fieldErrors['checkout.success_url']} optional>
            <Input type="url" value={draft.success_url} onChange={(e) => patch({ success_url: e.target.value })} disabled={readOnly} placeholder="https://" />
          </Field>
          <Field label={t('settings.checkout.cancelUrl')} hint={t('settings.checkout.cancelUrlHint')} error={fieldErrors['checkout.cancel_url']} optional>
            <Input type="url" value={draft.cancel_url} onChange={(e) => patch({ cancel_url: e.target.value })} disabled={readOnly} placeholder="https://" />
          </Field>
        </CardBody>
      </Card>

      {!readOnly && <SaveBar dirty={dirty} pending={pending} disabled={!numbersOk} onReset={() => setDraft(init(settings))} />}
    </form>
  );
}
