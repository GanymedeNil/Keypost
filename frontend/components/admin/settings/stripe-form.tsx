'use client';

import { useState, type FormEvent } from 'react';
import { ExternalLink } from 'lucide-react';
import { ButtonLink, Card, CardBody, CardHeader, CopyField, Field, Input, Select, SwitchField } from '@/components/kit';
import { useI18n } from '@/lib/i18n/provider';
import type { Role, Settings, SettingsUpdate } from '@/lib/types';
import { ChipInput } from './chip-input';
import { SaveBar } from './save-bar';
import { emptySecret, SecretField, secretUpdate, type SecretDraft } from './secret-field';
import type { SettingsCtx } from './settings-gate';

interface Draft {
  secret_key: SecretDraft;
  webhook_secret: SecretDraft;
  oauth_secret_key: SecretDraft;
  managed_payments: boolean;
  oauth_client_id: string;
  login_mode: string;
  login_accounts: string[];
  login_auto_role: Role;
  oauth_revoke_after_login: boolean;
}

function init(s: Settings): Draft {
  const st = s.stripe;
  return {
    secret_key: emptySecret,
    webhook_secret: emptySecret,
    oauth_secret_key: emptySecret,
    managed_payments: st.managed_payments,
    oauth_client_id: st.oauth_client_id ?? '',
    login_mode: st.login_mode,
    login_accounts: st.login_accounts ?? [],
    login_auto_role: st.login_auto_role,
    oauth_revoke_after_login: st.oauth_revoke_after_login,
  };
}

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((v, i) => v === b[i]);

/** Partial update: only fields that changed; secrets only when typed or cleared. */
function diff(s: Settings, d: Draft): NonNullable<SettingsUpdate['stripe']> {
  const st = s.stripe;
  const out: NonNullable<SettingsUpdate['stripe']> = {};
  const secretKey = secretUpdate(d.secret_key);
  if (secretKey !== undefined) out.secret_key = secretKey;
  const webhookSecret = secretUpdate(d.webhook_secret);
  if (webhookSecret !== undefined) out.webhook_secret = webhookSecret;
  const oauthSecret = secretUpdate(d.oauth_secret_key);
  if (oauthSecret !== undefined) out.oauth_secret_key = oauthSecret;
  if (d.managed_payments !== st.managed_payments) out.managed_payments = d.managed_payments;
  if (d.oauth_client_id !== (st.oauth_client_id ?? '')) out.oauth_client_id = d.oauth_client_id.trim();
  if (d.login_mode !== st.login_mode) out.login_mode = d.login_mode;
  if (!sameList(d.login_accounts, st.login_accounts ?? [])) out.login_accounts = d.login_accounts;
  if (d.login_auto_role !== st.login_auto_role) out.login_auto_role = d.login_auto_role;
  if (d.oauth_revoke_after_login !== st.oauth_revoke_after_login) out.oauth_revoke_after_login = d.oauth_revoke_after_login;
  return out;
}

export function StripeForm({ ctx }: { ctx: SettingsCtx }) {
  const { t, te } = useI18n();
  const { settings, readOnly, pending, fieldErrors, save } = ctx;
  const [draft, setDraft] = useState(() => init(settings));
  const update = diff(settings, draft);
  const dirty = Object.keys(update).length > 0;
  const patch = (p: Partial<Draft>) => setDraft((d) => ({ ...d, ...p }));
  const err = (key: string) => fieldErrors[`stripe.${key}`];
  const auto = draft.login_mode === 'auto';

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!dirty || readOnly) return;
    await save({ stripe: update });
  }

  return (
    <form onSubmit={submit} className="space-y-6" noValidate>
      <Card>
        <CardHeader title={t('settings.stripe.keys.title')} description={t('settings.stripe.keys.description')} />
        <CardBody className="space-y-5">
          <SecretField
            label={t('settings.stripe.secretKey')}
            current={settings.stripe.secret_key}
            draft={draft.secret_key}
            onChange={(secret_key) => patch({ secret_key })}
            disabled={readOnly}
            error={err('secret_key')}
          />
          <SecretField
            label={t('settings.stripe.webhookSecret')}
            current={settings.stripe.webhook_secret}
            draft={draft.webhook_secret}
            onChange={(webhook_secret) => patch({ webhook_secret })}
            disabled={readOnly}
            error={err('webhook_secret')}
          />
          <Field label={t('settings.stripe.apiVersion')}>
            <Input value={settings.stripe.api_version} readOnly className="font-mono" />
          </Field>
          <SwitchField
            label={t('settings.stripe.managedPayments')}
            description={t('settings.stripe.managedPaymentsDesc')}
            checked={draft.managed_payments}
            onCheckedChange={(managed_payments) => patch({ managed_payments })}
            disabled={readOnly}
          />
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title={t('settings.stripe.webhook.title')}
          description={t('settings.stripe.webhook.description')}
          actions={
            <ButtonLink href="/admin/stripe" size="sm" icon={<ExternalLink className="size-3.5" />}>
              {t('settings.stripe.webhook.open')}
            </ButtonLink>
          }
        />
        <CardBody>
          <Field label={t('settings.stripe.webhook.url')}>
            <CopyField value={settings.urls.stripe_webhook_url} label={t('settings.stripe.webhook.url')} />
          </Field>
        </CardBody>
      </Card>

      <Card>
        <CardHeader title={t('settings.stripe.login.title')} description={t('settings.stripe.login.description')} />
        <CardBody className="space-y-5">
          <div className="grid gap-5 sm:grid-cols-2">
            <Field label={t('settings.stripe.login.clientId')} error={err('oauth_client_id')}>
              <Input
                value={draft.oauth_client_id}
                onChange={(e) => patch({ oauth_client_id: e.target.value })}
                disabled={readOnly}
                placeholder="ca_…"
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
              />
            </Field>
            <SecretField
              label={t('settings.stripe.login.clientSecret')}
              current={settings.stripe.oauth_secret_key}
              draft={draft.oauth_secret_key}
              onChange={(oauth_secret_key) => patch({ oauth_secret_key })}
              disabled={readOnly}
              error={err('oauth_secret_key')}
            />
          </div>
          <Field label={t('settings.stripe.login.redirectUri')} hint={t('settings.stripe.login.redirectUriHint')}>
            <CopyField value={settings.urls.stripe_oauth_redirect_uri} label={t('settings.stripe.login.redirectUri')} />
          </Field>
          <div className="grid gap-5 sm:grid-cols-2">
            <Field
              label={t('settings.stripe.login.mode')}
              hint={te(`settings.stripe.login.modeHint.${draft.login_mode}`, '')}
              error={err('login_mode')}
            >
              <Select value={draft.login_mode} onChange={(e) => patch({ login_mode: e.target.value })} disabled={readOnly}>
                <option value="allowlist">{t('settings.stripe.login.mode.allowlist')}</option>
                <option value="auto">{t('settings.stripe.login.mode.auto')}</option>
              </Select>
            </Field>
            {auto && (
              <Field label={t('settings.stripe.login.autoRole')} hint={t('settings.stripe.login.autoRoleHint')} error={err('login_auto_role')}>
                <Select value={draft.login_auto_role} onChange={(e) => patch({ login_auto_role: e.target.value as Role })} disabled={readOnly}>
                  <option value="viewer">{t('role.viewer')}</option>
                  <option value="admin">{t('role.admin')}</option>
                </Select>
              </Field>
            )}
          </div>
          <Field
            label={t('settings.stripe.login.accounts')}
            hint={auto ? t('settings.stripe.login.accountsHintAuto') : t('settings.stripe.login.accountsHint')}
            error={err('login_accounts')}
          >
            <ChipInput
              values={draft.login_accounts}
              onChange={(login_accounts) => patch({ login_accounts })}
              validate={(v) => (v.startsWith('acct_') && v.length > 5 ? null : t('settings.stripe.login.accountInvalid'))}
              placeholder="acct_…"
              disabled={readOnly}
            />
          </Field>
          <SwitchField
            label={t('settings.stripe.login.revoke')}
            description={t('settings.stripe.login.revokeDesc')}
            checked={draft.oauth_revoke_after_login}
            onCheckedChange={(oauth_revoke_after_login) => patch({ oauth_revoke_after_login })}
            disabled={readOnly}
          />
        </CardBody>
      </Card>

      {!readOnly && <SaveBar dirty={dirty} pending={pending} onReset={() => setDraft(init(settings))} />}
    </form>
  );
}
