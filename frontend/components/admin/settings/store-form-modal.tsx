'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import type { Store, StoreInput } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { slugify } from '@/lib/utils';
import { Button, Field, Input, Modal, Textarea } from '@/components/ui';

interface Draft {
  name: string;
  slug: string;
  domain: string;
  url: string;
  avatar_url: string;
  country: string;
  currency: string;
  support_email: string;
  email_from_name: string;
  receipt_subject: string;
  receipt_intro: string;
}

function init(s: Store | null): Draft {
  return {
    name: s?.name ?? '',
    slug: s?.slug ?? '',
    domain: s?.domain ?? '',
    url: s?.url ?? '',
    avatar_url: s?.avatar_url ?? '',
    country: s?.country ?? '',
    currency: s?.currency ?? '',
    support_email: s?.support_email ?? '',
    email_from_name: s?.email_from_name ?? '',
    receipt_subject: s?.receipt_subject ?? '',
    receipt_intro: s?.receipt_intro ?? '',
  };
}

/** Create sends only filled fields (backend defaults apply); edit sends everything so fields can be cleared. */
function toBody(d: Draft, editing: boolean): StoreInput {
  const entries = Object.entries(d).map(([k, v]) => [k, v.trim()] as const);
  return Object.fromEntries(editing ? entries : entries.filter(([, v]) => v !== '')) as StoreInput;
}

export function StoreFormModal({ store, onClose, onSaved }: { store: Store | null; onClose: () => void; onSaved: () => Promise<unknown> }) {
  const { t } = useI18n();
  const editing = store !== null;
  const [draft, setDraft] = useState(() => init(store));
  const [slugTouched, setSlugTouched] = useState(editing);
  const { run, pending, fieldErrors } = useAction();
  const set = (k: keyof Draft) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const value = e.target.value;
    setDraft((d) => ({ ...d, [k]: value }));
  };

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!draft.name.trim()) return;
    const body = toBody(draft, editing);
    const res = await run(() => (store ? adminApi.patch(`/stores/${store.id}`, body) : adminApi.post('/stores', body)), {
      success: store ? t('toast.saved') : t('toast.created'),
    });
    if (res.ok) {
      await onSaved();
      onClose();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={editing ? t('settings.stores.edit') : t('settings.stores.new')}
      size="lg"
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="store-form" variant="primary" loading={pending} disabled={!draft.name.trim()}>
            {editing ? t('common.save') : t('settings.stores.create')}
          </Button>
        </>
      }
    >
      <form id="store-form" onSubmit={submit} className="space-y-6" noValidate>
        <fieldset className="space-y-4">
          <legend className="mb-3 text-xs font-semibold uppercase tracking-wide text-subtle">{t('settings.stores.section.general')}</legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('settings.stores.field.name')} error={fieldErrors.name} required>
              <Input
                value={draft.name}
                autoFocus
                onChange={(e) => {
                  const name = e.target.value;
                  setDraft((d) => ({ ...d, name, slug: slugTouched ? d.slug : slugify(name) }));
                }}
              />
            </Field>
            <Field label={t('settings.stores.field.slug')} hint={t('settings.stores.field.slugHint')} error={fieldErrors.slug}>
              <Input
                value={draft.slug}
                className="font-mono"
                onChange={(e) => {
                  setSlugTouched(true);
                  set('slug')(e);
                }}
              />
            </Field>
            <Field label={t('settings.stores.field.domain')} error={fieldErrors.domain} optional>
              <Input value={draft.domain} onChange={set('domain')} placeholder="shop.example.com" />
            </Field>
            <Field label={t('settings.stores.field.url')} error={fieldErrors.url} optional>
              <Input type="url" value={draft.url} onChange={set('url')} placeholder="https://shop.example.com" />
            </Field>
            <Field label={t('settings.stores.field.avatarUrl')} error={fieldErrors.avatar_url} optional>
              <Input type="url" value={draft.avatar_url} onChange={set('avatar_url')} placeholder="https://" />
            </Field>
            <Field label={t('settings.stores.field.supportEmail')} error={fieldErrors.support_email} optional>
              <Input type="email" value={draft.support_email} onChange={set('support_email')} />
            </Field>
            <Field label={t('settings.stores.field.country')} hint={t('settings.stores.field.countryHint')} error={fieldErrors.country} optional>
              <Input
                value={draft.country}
                maxLength={2}
                className="uppercase"
                onChange={(e) => setDraft((d) => ({ ...d, country: e.target.value.toUpperCase() }))}
              />
            </Field>
            <Field label={t('settings.stores.field.currency')} hint={t('settings.stores.field.currencyHint')} error={fieldErrors.currency} optional>
              <Input
                value={draft.currency}
                maxLength={3}
                className="uppercase"
                onChange={(e) => setDraft((d) => ({ ...d, currency: e.target.value.toUpperCase() }))}
              />
            </Field>
          </div>
        </fieldset>
        <fieldset className="space-y-4">
          <legend className="mb-3 text-xs font-semibold uppercase tracking-wide text-subtle">{t('settings.stores.section.receipt')}</legend>
          <Field label={t('settings.stores.field.emailFromName')} hint={t('settings.stores.field.emailFromNameHint')} error={fieldErrors.email_from_name} optional>
            <Input value={draft.email_from_name} onChange={set('email_from_name')} />
          </Field>
          <Field label={t('settings.stores.field.receiptSubject')} hint={t('settings.stores.field.receiptSubjectHint')} error={fieldErrors.receipt_subject} optional>
            <Input value={draft.receipt_subject} onChange={set('receipt_subject')} />
          </Field>
          <Field label={t('settings.stores.field.receiptIntro')} hint={t('settings.stores.field.receiptIntroHint')} error={fieldErrors.receipt_intro} optional>
            <Textarea rows={4} value={draft.receipt_intro} onChange={set('receipt_intro')} />
          </Field>
        </fieldset>
      </form>
    </Modal>
  );
}
