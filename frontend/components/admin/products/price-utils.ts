import type { useI18n } from '@/lib/i18n/provider';
import { formatMoney, majorToMinor, minorToMajor } from '@/lib/format';
import type { IntervalUnit, Price, PriceCategory, PriceInput } from '@/lib/types';

export type I18n = ReturnType<typeof useI18n>;

export const CATEGORIES: PriceCategory[] = ['one_time', 'subscription', 'lead_magnet', 'pwyw'];
export const INTERVALS: IntervalUnit[] = ['day', 'week', 'month', 'year'];

/** Form state for a price. Money fields are strings in MAJOR units (e.g. "9.99"). */
export interface PriceDraft {
  category: PriceCategory;
  currency: string;
  unit: string;
  setupEnabled: boolean;
  setup: string;
  intervalUnit: IntervalUnit;
  intervalQty: string;
  trialUnit: IntervalUnit;
  trialQty: string;
  min: string;
  suggested: string;
  taxCode: string;
}

function major(minor: number | null | undefined, currency: string): string {
  if (minor === null || minor === undefined || minor === 0) return '';
  return String(minorToMajor(minor, currency));
}

export function emptyDraft(currency: string): PriceDraft {
  return {
    category: 'one_time',
    currency,
    unit: '',
    setupEnabled: false,
    setup: '',
    intervalUnit: 'month',
    intervalQty: '1',
    trialUnit: 'day',
    trialQty: '',
    min: '',
    suggested: '',
    taxCode: '',
  };
}

export function priceToDraft(p: Price): PriceDraft {
  const cur = p.currency || 'USD';
  return {
    category: p.category,
    currency: cur,
    unit: major(p.unit_price, cur),
    setupEnabled: p.setup_fee_enabled,
    setup: major(p.setup_fee, cur),
    intervalUnit: p.renewal_interval_unit ?? 'month',
    intervalQty: String(p.renewal_interval_quantity ?? 1),
    trialUnit: p.trial_interval_unit ?? 'day',
    trialQty: p.trial_interval_quantity ? String(p.trial_interval_quantity) : '',
    min: major(p.min_price, cur),
    suggested: major(p.suggested_price, cur),
    taxCode: p.tax_code ?? '',
  };
}

export type DraftErrors = Partial<Record<keyof PriceDraft, true>>;

function parseMoney(value: string, currency: string): number | null {
  const trimmed = value.trim();
  if (trimmed === '') return 0;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n < 0) return null;
  return majorToMinor(n, currency);
}

function parseQty(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === '') return 0;
  const n = Number(trimmed);
  return Number.isInteger(n) && n >= 0 ? n : null;
}

/** Validate a draft client-side (cheap checks only; the API stays authoritative). */
export function validateDraft(d: PriceDraft): DraftErrors {
  const errs: DraftErrors = {};
  if (!/^[A-Za-z]{3}$/.test(d.currency.trim())) errs.currency = true;
  const cur = d.currency.trim().toUpperCase();
  if (d.category === 'one_time' || d.category === 'subscription') {
    if (parseMoney(d.unit, cur) === null) errs.unit = true;
  }
  if (d.category === 'pwyw') {
    if (parseMoney(d.min, cur) === null) errs.min = true;
    if (parseMoney(d.suggested, cur) === null) errs.suggested = true;
  }
  if (d.category === 'subscription') {
    const q = parseQty(d.intervalQty);
    if (q === null || q < 1) errs.intervalQty = true;
    if (parseQty(d.trialQty) === null) errs.trialQty = true;
    if (d.setupEnabled && parseMoney(d.setup, cur) === null) errs.setup = true;
  }
  return errs;
}

/**
 * Convert a validated draft to an API body. `scheme` is only sent for new prices
 * (existing non-standard schemes keep their tiers/package settings untouched).
 */
export function draftToInput(d: PriceDraft, creating: boolean): PriceInput {
  const currency = d.currency.trim().toUpperCase();
  const money = (v: string) => parseMoney(v, currency) ?? 0;
  const sub = d.category === 'subscription';
  const trialQty = sub ? (parseQty(d.trialQty) ?? 0) : 0;
  const body: PriceInput = {
    category: d.category,
    currency,
    unit_price: d.category === 'lead_magnet' ? 0 : d.category === 'pwyw' ? money(d.min) : money(d.unit),
    setup_fee_enabled: sub && d.setupEnabled,
    setup_fee: sub && d.setupEnabled ? money(d.setup) : 0,
    renewal_interval_unit: sub ? d.intervalUnit : null,
    renewal_interval_quantity: sub ? (parseQty(d.intervalQty) ?? 1) : null,
    trial_interval_unit: sub && trialQty > 0 ? d.trialUnit : null,
    trial_interval_quantity: sub && trialQty > 0 ? trialQty : null,
    min_price: d.category === 'pwyw' ? money(d.min) : 0,
    suggested_price: d.category === 'pwyw' ? money(d.suggested) : 0,
    tax_code: d.taxCode.trim() || null,
  };
  if (creating) body.scheme = 'standard';
  return body;
}

/** "/ month" or "every 3 months". */
export function intervalLabel(p: Pick<Price, 'renewal_interval_unit' | 'renewal_interval_quantity'>, i18n: I18n): string {
  const unit = p.renewal_interval_unit ?? 'month';
  const qty = p.renewal_interval_quantity ?? 1;
  return qty > 1 ? i18n.te(`products.perN.${unit}`, `/ ${qty} ${unit}`, { n: qty }) : i18n.te(`products.per.${unit}`, `/ ${unit}`);
}

export function trialLabel(p: Pick<Price, 'trial_interval_unit' | 'trial_interval_quantity'>, i18n: I18n): string | null {
  if (!p.trial_interval_unit || !p.trial_interval_quantity) return null;
  return i18n.te(`products.trial.${p.trial_interval_unit}`, `${p.trial_interval_quantity} ${p.trial_interval_unit} trial`, { n: p.trial_interval_quantity });
}

/** Compact one-line price description used in chips and tables. */
export function priceLabel(p: Price, i18n: I18n): string {
  const { t, locale } = i18n;
  switch (p.category) {
    case 'lead_magnet':
      return t('products.free');
    case 'pwyw':
      return t('products.pwywFrom', { amount: formatMoney(p.min_price ?? 0, p.currency, locale) });
    case 'subscription':
      return `${formatMoney(p.unit_price, p.currency, locale)} ${intervalLabel(p, i18n)}`;
    default:
      return formatMoney(p.unit_price, p.currency, locale);
  }
}

export const VARIANT_STATUSES = ['draft', 'published', 'pending'] as const;
