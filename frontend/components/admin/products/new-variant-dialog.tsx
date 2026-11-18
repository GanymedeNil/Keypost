'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useT, useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { slugify } from '@/lib/utils';
import type { Variant } from '@/lib/types';
import { Button, Field, Input, Modal, Select, Textarea } from '@/components/kit';
import { PriceFields } from './price-fields';
import { draftToInput, emptyDraft, validateDraft, VARIANT_STATUSES, type DraftErrors, type PriceDraft } from './price-utils';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  productId: number;
  currency: string;
  onCreated: (variant: Variant) => void | Promise<unknown>;
}

function NewVariantInner({ onOpenChange, productId, currency, onCreated }: Omit<Props, 'open'>) {
  const t = useT();
  const { te } = useI18n();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [slugEdited, setSlugEdited] = useState(false);
  const [description, setDescription] = useState('');
  const [status, setStatus] = useState<string>('draft');
  const [price, setPrice] = useState<PriceDraft>(() => emptyDraft(currency));
  const [errors, setErrors] = useState<DraftErrors>({});
  const { run, pending, fieldErrors } = useAction();
  const effectiveSlug = slugEdited ? slug : slugify(name);
  const formId = 'new-variant-form';

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs = validateDraft(price);
    setErrors(errs);
    if (Object.keys(errs).length > 0) return;
    const res = await run(
      () =>
        adminApi.post<Variant>('/variants', {
          product_id: productId,
          name: name.trim(),
          slug: effectiveSlug,
          description,
          status,
          price: draftToInput(price, true),
        }),
      { success: t('products.variant.created') },
    );
    if (res.ok) {
      await onCreated(res.data);
      onOpenChange(false);
    }
  }

  return (
    <Modal
      open
      onOpenChange={onOpenChange}
      dismissible={!pending}
      size="lg"
      title={t('products.variant.addTitle')}
      description={t('products.variant.addDesc')}
      footer={
        <>
          <Button onClick={() => onOpenChange(false)} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending}>
            {t('common.create')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('common.name')} required error={fieldErrors.name}>
            <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          </Field>
          <Field label={t('common.slug')} error={fieldErrors.slug}>
            <Input
              value={effectiveSlug}
              onChange={(e) => {
                setSlug(e.target.value);
                setSlugEdited(true);
              }}
            />
          </Field>
        </div>
        <Field label={t('common.description')} optional error={fieldErrors.description}>
          <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label={t('common.status')} error={fieldErrors.status}>
          <Select value={status} onChange={(e) => setStatus(e.target.value)}>
            {VARIANT_STATUSES.map((s) => (
              <option key={s} value={s}>
                {te(`status.${s}`, s)}
              </option>
            ))}
          </Select>
        </Field>
        <div className="border-t border-border pt-4">
          <h3 className="mb-3 text-sm font-semibold">{t('products.variant.initialPrice')}</h3>
          <PriceFields draft={price} onChange={(patch) => setPrice((d) => ({ ...d, ...patch }))} errors={errors} serverErrors={fieldErrors} />
        </div>
      </form>
    </Modal>
  );
}

export function NewVariantDialog(props: Props) {
  return props.open ? <NewVariantInner {...props} /> : null;
}
