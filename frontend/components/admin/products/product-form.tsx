'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import type { Product } from '@/lib/types';
import { Button, Card, CardBody, CardFooter, CardHeader, Field, Input, Select, SwitchField, Textarea } from '@/components/kit';

interface Draft {
  name: string;
  slug: string;
  description: string;
  status: string;
  thumb_url: string;
  large_thumb_url: string;
  stripe_product_id: string;
  test_mode: boolean;
}

function toDraft(p: Product): Draft {
  return {
    name: p.name,
    slug: p.slug,
    description: p.description ?? '',
    status: p.status,
    thumb_url: p.thumb_url ?? '',
    large_thumb_url: p.large_thumb_url ?? '',
    stripe_product_id: p.stripe_product_id ?? '',
    test_mode: p.test_mode,
  };
}

/** Inline editor for the product fields. Remount with `key={product.updated_at}` to reset after saves. */
export function ProductForm({ product, onSaved }: { product: Product; onSaved: () => Promise<unknown> }) {
  const { t, te } = useI18n();
  const can = useCan();
  const [draft, setDraft] = useState<Draft>(() => toDraft(product));
  const { run, pending, fieldErrors } = useAction();
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => ({ ...d, [k]: v }));
  const dirty = JSON.stringify(draft) !== JSON.stringify(toDraft(product));

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await run(
      () =>
        adminApi.patch<Product>(`/products/${product.id}`, {
          name: draft.name.trim(),
          slug: draft.slug.trim(),
          description: draft.description,
          status: draft.status,
          thumb_url: draft.thumb_url.trim(),
          large_thumb_url: draft.large_thumb_url.trim(),
          stripe_product_id: draft.stripe_product_id.trim() || null,
          test_mode: draft.test_mode,
        }),
      { success: t('toast.saved') },
    );
    if (res.ok) await onSaved();
  }

  return (
    <Card>
      <form onSubmit={submit} noValidate>
        <CardHeader title={t('products.form.title')} />
        <CardBody>
          <fieldset disabled={!can.write} className="min-w-0 space-y-4">
            <Field label={t('common.name')} required error={fieldErrors.name}>
              <Input value={draft.name} onChange={(e) => set('name', e.target.value)} />
            </Field>
            <Field label={t('common.slug')} error={fieldErrors.slug}>
              <Input value={draft.slug} onChange={(e) => set('slug', e.target.value)} />
            </Field>
            <Field label={t('common.description')} optional error={fieldErrors.description}>
              <Textarea rows={5} value={draft.description} onChange={(e) => set('description', e.target.value)} />
            </Field>
            <Field label={t('common.status')} error={fieldErrors.status}>
              <Select value={draft.status} onChange={(e) => set('status', e.target.value)}>
                <option value="draft">{te('status.draft', 'Draft')}</option>
                <option value="published">{te('status.published', 'Published')}</option>
              </Select>
            </Field>
            <Field label={t('products.form.thumbUrl')} optional error={fieldErrors.thumb_url}>
              <Input type="url" inputMode="url" placeholder="https://" value={draft.thumb_url} onChange={(e) => set('thumb_url', e.target.value)} />
            </Field>
            <Field label={t('products.form.largeThumbUrl')} optional error={fieldErrors.large_thumb_url}>
              <Input type="url" inputMode="url" placeholder="https://" value={draft.large_thumb_url} onChange={(e) => set('large_thumb_url', e.target.value)} />
            </Field>
            <Field label={t('products.form.stripeProductId')} hint={t('products.form.stripeProductIdHint')} optional error={fieldErrors.stripe_product_id}>
              <Input placeholder="prod_…" className="font-mono" value={draft.stripe_product_id} onChange={(e) => set('stripe_product_id', e.target.value)} />
            </Field>
            <SwitchField
              label={t('common.testMode')}
              description={t('products.form.testModeHint')}
              checked={draft.test_mode}
              onCheckedChange={(v) => set('test_mode', v)}
            />
          </fieldset>
        </CardBody>
        {can.write && (
          <CardFooter>
            {dirty && (
              <Button onClick={() => setDraft(toDraft(product))} disabled={pending}>
                {t('common.reset')}
              </Button>
            )}
            <Button type="submit" variant="primary" loading={pending} disabled={!dirty}>
              {t('common.saveChanges')}
            </Button>
          </CardFooter>
        )}
      </form>
    </Card>
  );
}
