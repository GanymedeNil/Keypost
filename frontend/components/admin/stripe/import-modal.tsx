'use client';

import { useState } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import { isRecord } from '@/lib/utils';
import { useAction } from '@/lib/use-action';
import { Button, Field, Modal, SwitchField, useToast } from '@/components/ui';
import { StoreSelect, useDefaultStoreId } from '@/components/admin/store-select';

/**
 * Import Stripe products into a store. `productIds` = selected products, `null` = every unmapped product
 * (the API treats a missing `stripe_product_ids` as "all"). The response shape is not pinned by the contract,
 * so only an optional numeric `imported` is read from it.
 */
export function ImportModal({ productIds, count, onClose, onImported }: { productIds: string[] | null; count: number; onClose: () => void; onImported: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const defaultStore = useDefaultStoreId();
  const [storeId, setStoreId] = useState<number | undefined>(defaultStore);
  const [publish, setPublish] = useState(true);
  const { run, pending, fieldErrors } = useAction();

  async function submit() {
    const res = await run(() =>
      adminApi.post<unknown>('/stripe/catalog/import', {
        store_id: storeId,
        publish,
        ...(productIds ? { stripe_product_ids: productIds } : {}),
      }),
    );
    if (!res.ok) return;
    const imported = isRecord(res.data) && typeof res.data.imported === 'number' ? res.data.imported : null;
    toast.success(imported === null ? t('stripe.catalog.importDone') : t('stripe.catalog.importDoneCount', { count: imported }));
    await invalidate('/products', '/variants', '/prices');
    onImported();
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('stripe.catalog.importTitle')}
      description={productIds ? t('stripe.catalog.importSelectedBody', { count }) : t('stripe.catalog.importAllBody')}
      size="md"
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={pending} disabled={!storeId} onClick={submit}>
            {pending ? t('stripe.catalog.importing') : t('stripe.catalog.confirmImport')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label={t('stripe.catalog.targetStore')} error={fieldErrors.store_id}>
          <StoreSelect value={storeId} onChange={setStoreId} />
        </Field>
        <SwitchField label={t('stripe.catalog.publish')} description={t('stripe.catalog.publishHint')} checked={publish} onCheckedChange={setPublish} />
      </div>
    </Modal>
  );
}
