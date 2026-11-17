/** Locale-aware formatting helpers. Money is always integer minor units + ISO currency. */

const digitsCache = new Map<string, number>();

function fractionDigits(currency: string, locale: string): number {
  const key = `${locale}:${currency}`;
  let d = digitsCache.get(key);
  if (d === undefined) {
    try {
      d = new Intl.NumberFormat(locale, { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
    } catch {
      d = 2;
    }
    digitsCache.set(key, d);
  }
  return d;
}

export function minorToMajor(minor: number, currency: string, locale = 'en'): number {
  return minor / 10 ** fractionDigits(currency, locale);
}

export function majorToMinor(major: number, currency: string, locale = 'en'): number {
  return Math.round(major * 10 ** fractionDigits(currency, locale));
}

export function formatMoney(
  minor: number | null | undefined,
  currency: string | null | undefined,
  locale = 'en',
  opts: { compact?: boolean } = {},
): string {
  if (minor === null || minor === undefined || Number.isNaN(minor)) return '—';
  const cur = (currency || 'USD').toUpperCase();
  const value = minorToMajor(minor, cur, locale);
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: cur,
      ...(opts.compact ? { notation: 'compact', maximumFractionDigits: 1 } : {}),
    }).format(value);
  } catch {
    return `${value.toFixed(2)} ${cur}`;
  }
}

export function formatNumber(n: number | null | undefined, locale = 'en', opts: Intl.NumberFormatOptions = {}): string {
  if (n === null || n === undefined || Number.isNaN(n)) return '—';
  return new Intl.NumberFormat(locale, opts).format(n);
}

export function formatPercent(n: number | null | undefined, locale = 'en'): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return '—';
  return new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 1, signDisplay: 'exceptZero' }).format(n / 100);
}

function toDate(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null;
  const d = value instanceof Date ? value : new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function formatDate(
  value: string | number | Date | null | undefined,
  locale = 'en',
  style: 'date' | 'datetime' | 'time' | 'short' = 'datetime',
): string {
  const d = toDate(value);
  if (!d) return '—';
  const opts: Intl.DateTimeFormatOptions =
    style === 'date'
      ? { year: 'numeric', month: 'short', day: 'numeric' }
      : style === 'short'
        ? { month: 'short', day: 'numeric' }
        : style === 'time'
          ? { hour: '2-digit', minute: '2-digit', second: '2-digit' }
          : { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' };
  return new Intl.DateTimeFormat(locale, opts).format(d);
}

export function formatRelative(value: string | number | Date | null | undefined, locale = 'en', now = Date.now()): string {
  const d = toDate(value);
  if (!d) return '—';
  const diff = (d.getTime() - now) / 1000;
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (abs < 45) return rtf.format(Math.round(diff), 'second');
  if (abs < 2700) return rtf.format(Math.round(diff / 60), 'minute');
  if (abs < 79200) return rtf.format(Math.round(diff / 3600), 'hour');
  if (abs < 2_246_400) return rtf.format(Math.round(diff / 86400), 'day');
  if (abs < 27_648_000) return rtf.format(Math.round(diff / 2_592_000), 'month');
  return rtf.format(Math.round(diff / 31_536_000), 'year');
}

export function formatBytes(bytes: number | null | undefined, locale = 'en'): string {
  if (bytes === null || bytes === undefined) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let n = bytes;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: i === 0 ? 0 : 1 }).format(n)} ${units[i]}`;
}

/** `2026-11-17T09:30:00Z` -> value usable by <input type="datetime-local"> in local time. */
export function toLocalInput(value: string | null | undefined): string {
  const d = toDate(value);
  if (!d) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Inverse of {@link toLocalInput}: local datetime-local value -> RFC3339 UTC (or null when empty). */
export function fromLocalInput(value: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/** `<input type="date">` value -> RFC3339 at local start/end of day, for created_from / created_to filters. */
export function dateToRange(value: string, edge: 'start' | 'end'): string | undefined {
  if (!value) return undefined;
  const d = new Date(`${value}T${edge === 'start' ? '00:00:00' : '23:59:59'}`);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

export function rangeToDate(value: string | null | undefined): string {
  const d = toDate(value);
  if (!d) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
