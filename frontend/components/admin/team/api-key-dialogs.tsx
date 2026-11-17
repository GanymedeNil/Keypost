'use client';

import { useId, useState, type FormEvent } from 'react';
import { Check, Copy } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { fromLocalInput } from '@/lib/format';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import type { ApiKeyCreated } from '@/lib/types';
import { Button, Callout, Field, Input, Modal, SwitchField, copyText, useConfirm, useToast } from '@/components/ui';
import { useAdmin } from '@/components/admin/admin-context';
import { useFieldErrors } from './use-field-errors';

export function CreateKeyDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (key: ApiKeyCreated) => void }) {
  const t = useT();
  const { testMode } = useAdmin();
  const formId = useId();
  const [name, setName] = useState('');
  const [test, setTest] = useState(testMode);
  const [expires, setExpires] = useState('');
  const { run, pending, fieldErrors } = useAction();
  const { errors, touch, reset } = useFieldErrors(fieldErrors);

  async function submit(e: FormEvent) {
    e.preventDefault();
    reset();
    const res = await run(() =>
      adminApi.post<ApiKeyCreated>('/api-keys', {
        name: name.trim(),
        test_mode: test,
        expires_at: fromLocalInput(expires) ?? undefined,
      }),
    );
    if (res.ok) onCreated(res.data);
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && !pending && onClose()}
      title={t('team.apiKeys.create')}
      description={t('team.apiKeys.createDesc')}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending} disabled={!name.trim()}>
            {t('common.create')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} className="space-y-4" noValidate>
        <Field label={t('common.name')} error={errors.name} required>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              touch('name');
            }}
            placeholder={t('team.apiKeys.namePlaceholder')}
            autoFocus
            maxLength={80}
          />
        </Field>
        <SwitchField label={t('team.apiKeys.testMode')} description={t('team.apiKeys.testModeHint')} checked={test} onCheckedChange={setTest} />
        <Field label={t('team.apiKeys.expires')} hint={t('team.apiKeys.expiresHint')} error={errors.expires_at} optional>
          <Input
            type="datetime-local"
            value={expires}
            onChange={(e) => {
              setExpires(e.target.value);
              touch('expires_at');
            }}
          />
        </Field>
      </form>
    </Modal>
  );
}

/** Shows the freshly created token once. Closing before copying asks for confirmation. */
export function TokenDialog({ apiKey, onClose }: { apiKey: ApiKeyCreated; onClose: () => void }) {
  const t = useT();
  const toast = useToast();
  const confirm = useConfirm();
  const [copied, setCopied] = useState(false);

  async function copy() {
    if (await copyText(apiKey.token)) setCopied(true);
    else toast.error(t('toast.copyFailed'));
  }

  async function requestClose() {
    if (copied) return onClose();
    const ok = await confirm({
      title: t('team.apiKeys.tokenConfirmTitle'),
      description: t('team.apiKeys.tokenConfirmBody'),
      confirmLabel: t('team.apiKeys.tokenConfirmLabel'),
    });
    if (ok) onClose();
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && void requestClose()}
      title={t('team.apiKeys.tokenTitle')}
      description={t('team.apiKeys.tokenDesc')}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t('team.apiKeys.tokenDone')}
        </Button>
      }
    >
      <div className="space-y-4">
        <Callout tone="warning">{t('team.apiKeys.tokenWarn')}</Callout>
        <div className="space-y-1.5">
          <p className="text-xs font-medium">{apiKey.name}</p>
          <div className="flex items-stretch gap-2">
            <code
              className="min-w-0 flex-1 select-all break-all rounded-md border border-line-strong bg-surface-2 px-2.5 py-2 text-xs leading-relaxed"
              data-testid="new-api-token"
            >
              {apiKey.token}
            </code>
            <Button onClick={copy} icon={copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />} className="h-auto">
              {copied ? t('common.copied') : t('team.apiKeys.tokenCopy')}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
