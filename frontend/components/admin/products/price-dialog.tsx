'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import type { Price } from '@/lib/types';
import { Button, Modal } from '@/components/ui';
import { PriceFields } from './price-fields';
import { draftToInput, emptyDraft, priceToDraft, validateDraft, type DraftErrors, type PriceDraft } from './price-utils';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variantId: number;
  /** Default currency for new prices. */
  currency: string;
  /** Edit this price; omit to create. */
  price?: Price;
  onSaved: () => void | Promise<unknown>;
}

function PriceDialogInner({ onOpenChange, variantId, currency, price, onSaved }: Omit<Props, 'open'>) {
  const t = useT();
  const [draft, setDraft] = useState<PriceDraft>(() => (price ? priceToDraft(price) : emptyDraft(currency)));
  const [errors, setErrors] = useState<DraftErrors>({});
  const { run, pending, fieldErrors } = useAction();
  const formId = 'price-form';

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs = validateDraft(draft);
    setErrors(errs);
    if (Object.keys(errs).length > 0) return;
    const body = draftToInput(draft, !price);
    const res = await run(
      () => (price ? adminApi.patch<Price>(`/prices/${price.id}`, body) : adminApi.post<Price>('/prices', { variant_id: variantId, ...body })),
      { success: t('products.price.saved') },
    );
    if (res.ok) {
      await onSaved();
      onOpenChange(false);
    }
  }

  return (
    <Modal
      open
      onOpenChange={onOpenChange}
      dismissible={!pending}
      title={price ? t('products.price.editTitle') : t('products.price.addTitle')}
      footer={
        <>
          <Button onClick={() => onOpenChange(false)} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate>
        <PriceFields
          draft={draft}
          onChange={(patch) => setDraft((d) => ({ ...d, ...patch }))}
          errors={errors}
          serverErrors={fieldErrors}
          nonStandardScheme={price?.scheme}
        />
      </form>
    </Modal>
  );
}

/** Add/edit price dialog. Mounted only while open so the form state resets every time. */
export function PriceDialog(props: Props) {
  return props.open ? <PriceDialogInner {...props} /> : null;
}
