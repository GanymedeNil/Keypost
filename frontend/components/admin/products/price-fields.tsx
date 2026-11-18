'use client';

import { useT, useI18n } from '@/lib/i18n/provider';
import { Field, Input, InputGroup, Select, SwitchField } from '@/components/kit';
import { CATEGORIES, INTERVALS, type DraftErrors, type PriceDraft } from './price-utils';

/** The price form shared by the "add price" dialog and the "add variant" dialog. */
export function PriceFields({
  draft,
  onChange,
  errors = {},
  serverErrors = {},
  nonStandardScheme,
}: {
  draft: PriceDraft;
  onChange: (patch: Partial<PriceDraft>) => void;
  errors?: DraftErrors;
  /** Validation errors returned by the API (keys like `unit_price` or `price.unit_price`). */
  serverErrors?: Record<string, string>;
  /** Existing price uses a tiered/package scheme: show a read-only note. */
  nonStandardScheme?: string | null;
}) {
  const t = useT();
  const { te } = useI18n();
  const invalid = t('products.price.invalid');
  const err = (draftKey: keyof PriceDraft, ...apiKeys: string[]) => {
    for (const k of apiKeys) {
      const v = serverErrors[k] ?? serverErrors[`price.${k}`];
      if (v) return v;
    }
    return errors[draftKey] ? invalid : undefined;
  };
  const sub = draft.category === 'subscription';

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('products.price.category')} error={err('category', 'category')}>
          <Select value={draft.category} onChange={(e) => onChange({ category: e.target.value as PriceDraft['category'] })}>
            {CATEGORIES.map((c) => (
              <option key={c} value={c}>
                {te(`products.category.${c}`, c)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('common.currency')} hint={t('products.price.currencyHint')} error={err('currency', 'currency')}>
          <Input
            value={draft.currency}
            maxLength={3}
            className="uppercase"
            onChange={(e) => onChange({ currency: e.target.value.toUpperCase() })}
          />
        </Field>
      </div>

      {(draft.category === 'one_time' || sub) && (
        <Field label={t('products.price.amount')} error={err('unit', 'unit_price')}>
          <InputGroup
            inputMode="decimal"
            placeholder="0.00"
            suffix={draft.currency.toUpperCase()}
            value={draft.unit}
            onChange={(e) => onChange({ unit: e.target.value })}
          />
        </Field>
      )}

      {draft.category === 'lead_magnet' && <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">{t('products.price.leadMagnetHint')}</p>}

      {draft.category === 'pwyw' && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('products.price.minPrice')} hint={t('products.price.minPriceHint')} error={err('min', 'min_price')}>
            <InputGroup
              inputMode="decimal"
              placeholder="0.00"
              suffix={draft.currency.toUpperCase()}
              value={draft.min}
              onChange={(e) => onChange({ min: e.target.value })}
            />
          </Field>
          <Field label={t('products.price.suggestedPrice')} error={err('suggested', 'suggested_price')}>
            <InputGroup
              inputMode="decimal"
              placeholder="0.00"
              suffix={draft.currency.toUpperCase()}
              value={draft.suggested}
              onChange={(e) => onChange({ suggested: e.target.value })}
            />
          </Field>
        </div>
      )}

      {sub && (
        <>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('products.price.billEvery')} error={err('intervalQty', 'renewal_interval_quantity')}>
              <Input type="number" min={1} step={1} value={draft.intervalQty} onChange={(e) => onChange({ intervalQty: e.target.value })} />
            </Field>
            <Field label={t('products.price.interval')} error={err('intervalUnit', 'renewal_interval_unit')}>
              <Select value={draft.intervalUnit} onChange={(e) => onChange({ intervalUnit: e.target.value as PriceDraft['intervalUnit'] })}>
                {INTERVALS.map((u) => (
                  <option key={u} value={u}>
                    {te(`products.unit.${u}`, u)}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('products.price.trialLength')} hint={t('products.price.trialHint')} error={err('trialQty', 'trial_interval_quantity')}>
              <Input type="number" min={0} step={1} placeholder="0" value={draft.trialQty} onChange={(e) => onChange({ trialQty: e.target.value })} />
            </Field>
            <Field label={t('products.price.trialUnit')} error={err('trialUnit', 'trial_interval_unit')}>
              <Select
                value={draft.trialUnit}
                disabled={!draft.trialQty || draft.trialQty === '0'}
                onChange={(e) => onChange({ trialUnit: e.target.value as PriceDraft['trialUnit'] })}
              >
                {INTERVALS.map((u) => (
                  <option key={u} value={u}>
                    {te(`products.unit.${u}`, u)}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <SwitchField
            label={t('products.price.setupFee')}
            description={t('products.price.setupFeeHint')}
            checked={draft.setupEnabled}
            onCheckedChange={(v) => onChange({ setupEnabled: v })}
          />
          {draft.setupEnabled && (
            <Field label={t('products.price.setupFeeAmount')} error={err('setup', 'setup_fee')}>
              <InputGroup
                inputMode="decimal"
                placeholder="0.00"
                suffix={draft.currency.toUpperCase()}
                value={draft.setup}
                onChange={(e) => onChange({ setup: e.target.value })}
              />
            </Field>
          )}
        </>
      )}

      <Field label={t('products.price.taxCode')} hint={t('products.price.taxCodeHint')} optional error={err('taxCode', 'tax_code')}>
        <Input value={draft.taxCode} onChange={(e) => onChange({ taxCode: e.target.value })} placeholder="eservice" />
      </Field>

      {nonStandardScheme && nonStandardScheme !== 'standard' && (
        <p className="rounded-md border border-warning/30 bg-warning-soft px-3 py-2 text-xs text-warning-text">
          {t('products.price.schemeNote', { scheme: nonStandardScheme })}
        </p>
      )}
    </div>
  );
}
