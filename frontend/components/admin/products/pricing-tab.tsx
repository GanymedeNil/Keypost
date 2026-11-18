'use client';

import { useState } from 'react';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { adminApi, ApiError } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { errorText } from '@/lib/error-text';
import { useCan } from '@/components/admin/admin-context';
import type { Price, Variant } from '@/lib/types';
import { Badge, Button, EmptyState, Mono, MoneyText, useConfirmDelete, useToast } from '@/components/kit';
import { PriceDialog } from './price-dialog';
import { intervalLabel, trialLabel } from './price-utils';

export function PricingTab({ variant, currency, onChanged }: { variant: Variant; currency: string; onChanged: () => Promise<unknown> }) {
  const i18n = useI18n();
  const { t, te } = i18n;
  const can = useCan();
  const toast = useToast();
  const confirmDelete = useConfirmDelete();
  const { run } = useAction();
  const [dialog, setDialog] = useState<{ price?: Price } | null>(null);

  async function remove(price: Price) {
    if (!(await confirmDelete(te(`products.category.${price.category}`, price.category), t('products.price.deleteBody')))) return;
    const res = await run(() => adminApi.delete(`/prices/${price.id}`), { success: t('toast.deleted'), silent: true });
    if (res.ok) await onChanged();
    else if (res.error instanceof ApiError && res.error.code === 'conflict') toast.error(t('products.price.deleteConflict'));
    else toast.error(errorText(res.error, t));
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">{t('products.pricing.hint')}</p>
        {can.write && (
          <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => setDialog({})}>
            {t('products.price.add')}
          </Button>
        )}
      </div>

      {variant.prices.length === 0 ? (
        <div className="rounded-lg border border-dashed border-input">
          <EmptyState
            className="py-8"
            title={t('products.pricing.empty')}
            description={t('products.pricing.emptyHint')}
            action={
              can.write ? (
                <Button size="sm" variant="primary" onClick={() => setDialog({})}>
                  {t('products.price.add')}
                </Button>
              ) : undefined
            }
          />
        </div>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {variant.prices.map((p) => {
            const trial = trialLabel(p, i18n);
            return (
              <li key={p.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-3 py-2.5">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone={p.category === 'subscription' ? 'accent' : p.category === 'pwyw' ? 'info' : 'neutral'}>
                      {te(`products.category.${p.category}`, p.category)}
                    </Badge>
                    {p.category === 'pwyw' ? (
                      <span className="text-sm">
                        <MoneyText amount={p.min_price} currency={p.currency} className="font-medium" />
                        <span className="text-muted-foreground"> {t('products.price.minShort')}</span>
                        {!!p.suggested_price && (
                          <span className="text-muted-foreground">
                            {' · '}
                            <MoneyText amount={p.suggested_price} currency={p.currency} /> {t('products.price.suggestedShort')}
                          </span>
                        )}
                      </span>
                    ) : p.category === 'lead_magnet' ? (
                      <span className="text-sm font-medium">{t('products.free')}</span>
                    ) : (
                      <span className="text-sm">
                        <MoneyText amount={p.unit_price} currency={p.currency} className="font-medium" />
                        {p.category === 'subscription' && <span className="text-muted-foreground"> {intervalLabel(p, i18n)}</span>}
                      </span>
                    )}
                    {trial && <Badge tone="success">{trial}</Badge>}
                    {p.setup_fee_enabled && !!p.setup_fee && (
                      <Badge tone="warning">
                        {t('products.price.setupFeeBadge')} <MoneyText amount={p.setup_fee} currency={p.currency} />
                      </Badge>
                    )}
                    {p.scheme !== 'standard' && <Badge>{p.scheme}</Badge>}
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span>{p.currency}</span>
                    {p.stripe_price_id ? <Mono>{p.stripe_price_id}</Mono> : <Badge tone="warning">{t('products.stripe.notLinked')}</Badge>}
                  </div>
                </div>
                {can.write && (
                  <div className="flex items-center gap-1">
                    <Button size="icon-sm" variant="ghost" aria-label={t('common.edit')} onClick={() => setDialog({ price: p })}>
                      <Pencil className="size-3.5" />
                    </Button>
                    <Button size="icon-sm" variant="ghost" aria-label={t('common.delete')} className="hover:text-danger-text" onClick={() => remove(p)}>
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}

      <PriceDialog
        open={dialog !== null}
        onOpenChange={(o) => !o && setDialog(null)}
        variantId={variant.id}
        currency={currency}
        price={dialog?.price}
        onSaved={onChanged}
      />
    </div>
  );
}
