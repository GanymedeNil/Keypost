'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import type { Variant } from '@/lib/types';
import { Badge, Button, Callout, CopyField, Field, Input, Mono, useConfirm } from '@/components/kit';

export function StripeTab({ variant, onChanged }: { variant: Variant; onChanged: () => Promise<unknown> }) {
  const { t, te } = useI18n();
  const can = useCan();
  const confirm = useConfirm();
  const [productId, setProductId] = useState(variant.stripe_product_id ?? '');
  const [priceId, setPriceId] = useState(variant.prices.find((p) => p.stripe_price_id)?.stripe_price_id ?? '');
  const link = useAction();
  const unlink = useAction();
  const linked = !!variant.stripe_product_id;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await link.run(
      () => adminApi.post(`/variants/${variant.id}/link-stripe`, { stripe_product_id: productId.trim(), stripe_price_id: priceId.trim() }),
      { success: t('products.stripe.linked') },
    );
    if (res.ok) await onChanged();
  }

  async function doUnlink() {
    const ok = await confirm({
      title: t('products.stripe.unlinkTitle'),
      description: t('products.stripe.unlinkBody'),
      confirmLabel: t('common.unlink'),
      tone: 'danger',
    });
    if (!ok) return;
    const res = await unlink.run(() => adminApi.delete(`/variants/${variant.id}/link-stripe`), { success: t('products.stripe.unlinked') });
    if (res.ok) {
      setProductId('');
      setPriceId('');
      await onChanged();
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={linked ? 'success' : 'warning'} dot>
          {linked ? t('products.stripe.mapped') : t('products.stripe.notMapped')}
        </Badge>
        {linked && <Mono>{variant.stripe_product_id}</Mono>}
      </div>

      <form onSubmit={submit} noValidate>
        <fieldset disabled={!can.write} className="min-w-0 space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('products.stripe.productId')} error={link.fieldErrors.stripe_product_id}>
              <Input value={productId} onChange={(e) => setProductId(e.target.value)} placeholder="prod_…" className="font-mono" />
            </Field>
            <Field label={t('products.stripe.priceId')} hint={t('products.stripe.priceIdHint')} error={link.fieldErrors.stripe_price_id}>
              <Input value={priceId} onChange={(e) => setPriceId(e.target.value)} placeholder="price_…" className="font-mono" />
            </Field>
          </div>
        </fieldset>
        {can.write && (
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Button type="submit" variant="primary" loading={link.pending} disabled={!productId.trim() || !priceId.trim()}>
              {t('products.stripe.link')}
            </Button>
            {linked && (
              <Button variant="danger-outline" loading={unlink.pending} onClick={doUnlink}>
                {t('products.stripe.unlink')}
              </Button>
            )}
          </div>
        )}
      </form>

      {variant.prices.length > 0 && (
        <div>
          <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t('products.stripe.priceMapping')}</h3>
          <ul className="divide-y divide-border rounded-lg border border-border text-sm">
            {variant.prices.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                <span className="text-muted-foreground">
                  #{p.id} · {te(`products.category.${p.category}`, p.category)} · {p.currency}
                </span>
                {p.stripe_price_id ? <Mono>{p.stripe_price_id}</Mono> : <Badge tone="warning">{t('products.stripe.notLinked')}</Badge>}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t('products.stripe.buyUrl')}</h3>
        {variant.buy_now_url ? (
          <CopyField value={variant.buy_now_url} label={t('products.stripe.buyUrl')} />
        ) : (
          <p className="text-xs text-muted-foreground">{t('products.stripe.buyUrlMissing')}</p>
        )}
        <p className="mt-1.5 text-xs text-muted-foreground">{t('products.stripe.buyUrlHint')}</p>
      </div>

      <Callout tone="info">
        {t('products.stripe.catalogHint')}{' '}
        <Link href="/admin/stripe?tab=catalog" className="font-medium underline underline-offset-2">
          {t('products.stripe.openCatalog')}
        </Link>
      </Callout>
    </div>
  );
}
