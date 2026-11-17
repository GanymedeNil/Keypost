'use client';

import { useState, type FormEvent } from 'react';
import { Wand2 } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { Webhook } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { Button, CopyButton, Field, Input, Modal, SwitchField } from '@/components/ui';
import { useAdmin } from '@/components/admin/admin-context';
import { StoreSelect, useDefaultStoreId } from '@/components/admin/store-select';
import { EventPicker } from './event-picker';
import { useWebhookEvents } from './use-webhook-events';

const SECRET_ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';

function generateSecret(length = 32): string {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => SECRET_ALPHABET[b % SECRET_ALPHABET.length]).join('');
}

function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
}

/**
 * Create / edit dialog. Mount it only while open (`{editing && <WebhookFormModal …/>}`) so the form state
 * is initialised from `webhook` on every open without effects.
 */
export function WebhookFormModal({ webhook, onClose, onSaved }: { webhook: Webhook | null; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n();
  const { testMode } = useAdmin();
  const defaultStore = useDefaultStoreId();
  const events = useWebhookEvents();
  const { run, pending, fieldErrors } = useAction();

  const [storeId, setStoreId] = useState<number | undefined>(webhook?.store_id ?? defaultStore);
  const [url, setUrl] = useState(webhook?.url ?? '');
  const [selected, setSelected] = useState<string[]>(webhook?.events ?? []);
  const [secret, setSecret] = useState('');
  const [disabled, setDisabled] = useState(webhook?.disabled ?? false);
  const [test, setTest] = useState(webhook?.test_mode ?? testMode);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const err = (k: string) => errors[k] ?? fieldErrors[k];
  const clearErr = (k: string) => setErrors((cur) => (k in cur ? Object.fromEntries(Object.entries(cur).filter(([key]) => key !== k)) : cur));

  async function submit(e: FormEvent) {
    e.preventDefault();
    const local: Record<string, string> = {};
    if (!isHttpUrl(url.trim())) local.url = t('webhooks.err.url');
    if (selected.length === 0) local.events = t('webhooks.err.events');
    if (secret && (secret.length < 6 || secret.length > 40)) local.secret = t('webhooks.err.secret');
    setErrors(local);
    if (Object.keys(local).length > 0) return;

    const body = {
      url: url.trim(),
      events: selected,
      disabled,
      test_mode: test,
      ...(secret ? { secret } : {}),
    };
    const res = await run(
      () => (webhook ? adminApi.patch<Webhook>(`/webhooks/${webhook.id}`, body) : adminApi.post<Webhook>('/webhooks', { ...body, store_id: storeId })),
      { success: webhook ? t('webhooks.updated') : t('webhooks.created') },
    );
    if (res.ok) {
      await invalidate('/webhooks');
      onSaved();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={webhook ? t('webhooks.form.editTitle') : t('webhooks.form.createTitle')}
      size="lg"
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="webhook-form" variant="primary" loading={pending}>
            {pending ? t('common.saving') : webhook ? t('common.saveChanges') : t('common.create')}
          </Button>
        </>
      }
    >
      <form id="webhook-form" onSubmit={submit} noValidate className="space-y-4">
        {!webhook && (
          <Field label={t('webhooks.form.store')} error={err('store_id')}>
            <StoreSelect value={storeId} onChange={setStoreId} />
          </Field>
        )}
        <Field label={t('webhooks.form.url')} hint={t('webhooks.form.urlHint')} error={err('url')} required>
          <Input
            type="url"
            inputMode="url"
            autoFocus
            autoComplete="off"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              clearErr('url');
            }}
            placeholder={t('webhooks.form.urlPlaceholder')}
          />
        </Field>
        <Field label={t('webhooks.form.events')} hint={t('webhooks.form.eventsHint')} error={err('events')} required>
          <EventPicker
            events={events.data}
            loading={events.isLoading}
            error={!!events.error}
            value={selected}
            onChange={(next) => {
              setSelected(next);
              clearErr('events');
            }}
            invalid={!!err('events')}
          />
        </Field>
        <Field
          label={t('webhooks.form.secret')}
          hint={webhook?.secret_set ? t('webhooks.form.secretKeepHint') : t('webhooks.form.secretHint')}
          error={err('secret')}
          aside={
            <div className="flex items-center gap-1">
              {secret && <CopyButton value={secret} label={t('webhooks.form.secret')} />}
              <Button size="sm" variant="ghost" icon={<Wand2 className="size-3.5" />} onClick={() => {
                  setSecret(generateSecret());
                  clearErr('secret');
                }}>
                {t('webhooks.form.generate')}
              </Button>
            </div>
          }
        >
          <Input
            value={secret}
            onChange={(e) => {
              setSecret(e.target.value);
              clearErr('secret');
            }}
            autoComplete="off"
            spellCheck={false}
            maxLength={40}
            className="font-mono"
            placeholder={webhook?.secret_set ? t('webhooks.form.secretPlaceholderKeep') : undefined}
          />
        </Field>
        <div className="space-y-3 rounded-lg border border-line p-3">
          <SwitchField label={t('webhooks.form.disabled')} description={t('webhooks.form.disabledHint')} checked={disabled} onCheckedChange={setDisabled} />
          <SwitchField label={t('webhooks.form.testMode')} description={t('webhooks.form.testModeHint')} checked={test} onCheckedChange={setTest} />
        </div>
      </form>
    </Modal>
  );
}
