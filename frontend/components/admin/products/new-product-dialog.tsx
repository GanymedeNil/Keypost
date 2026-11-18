'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { slugify } from '@/lib/utils';
import type { Product } from '@/lib/types';
import { Button, Field, Input, Modal, Select, SwitchField, Textarea } from '@/components/kit';
import { useAdmin } from '@/components/admin/admin-context';
import { StoreSelect, useDefaultStoreId } from '@/components/admin/store-select';

function Inner({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  const { t, te } = useI18n();
  const router = useRouter();
  const { testMode: globalTestMode, stores } = useAdmin();
  const defaultStore = useDefaultStoreId();
  const [storeId, setStoreId] = useState<number | undefined>(defaultStore);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [slugEdited, setSlugEdited] = useState(false);
  const [description, setDescription] = useState('');
  const [status, setStatus] = useState('draft');
  const [thumb, setThumb] = useState('');
  const [testMode, setTestMode] = useState(globalTestMode);
  const { run, pending, fieldErrors } = useAction();
  const effectiveSlug = slugEdited ? slug : slugify(name);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await run(
      () =>
        adminApi.post<Product>('/products', {
          store_id: storeId,
          name: name.trim(),
          slug: effectiveSlug,
          description,
          status,
          thumb_url: thumb.trim(),
          test_mode: testMode,
        }),
      { success: t('products.created') },
    );
    if (res.ok) {
      onOpenChange(false);
      router.push(`/admin/products/${res.data.id}`);
    }
  }

  return (
    <Modal
      open
      onOpenChange={onOpenChange}
      dismissible={!pending}
      title={t('products.new.title')}
      description={t('products.new.desc')}
      footer={
        <>
          <Button onClick={() => onOpenChange(false)} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="new-product-form" variant="primary" loading={pending}>
            {t('common.create')}
          </Button>
        </>
      }
    >
      <form id="new-product-form" onSubmit={submit} noValidate className="space-y-4">
        {stores.length > 1 && (
          <Field label={t('common.store')} error={fieldErrors.store_id}>
            <StoreSelect value={storeId} onChange={setStoreId} />
          </Field>
        )}
        <Field label={t('common.name')} required error={fieldErrors.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <Field label={t('common.slug')} hint={t('products.new.slugHint')} error={fieldErrors.slug}>
          <Input
            value={effectiveSlug}
            onChange={(e) => {
              setSlug(e.target.value);
              setSlugEdited(true);
            }}
          />
        </Field>
        <Field label={t('common.description')} optional error={fieldErrors.description}>
          <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('common.status')} error={fieldErrors.status}>
            <Select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="draft">{te('status.draft', 'Draft')}</option>
              <option value="published">{te('status.published', 'Published')}</option>
            </Select>
          </Field>
          <Field label={t('products.form.thumbUrl')} optional error={fieldErrors.thumb_url}>
            <Input type="url" inputMode="url" placeholder="https://" value={thumb} onChange={(e) => setThumb(e.target.value)} />
          </Field>
        </div>
        <SwitchField label={t('common.testMode')} description={t('products.form.testModeHint')} checked={testMode} onCheckedChange={setTestMode} />
      </form>
    </Modal>
  );
}

export function NewProductDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return open ? <Inner onOpenChange={onOpenChange} /> : null;
}
