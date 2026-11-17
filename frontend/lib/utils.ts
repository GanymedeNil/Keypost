import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}

/** Turn `partial_refund` / `on_trial` into `Partial refund` as a last-resort label. */
export function humanize(value: string): string {
  const s = value.replace(/[_-]+/g, ' ').trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function slugify(value: string): string {
  return value
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^\p{Letter}\p{Number}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}

export function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Pretty-print JSON text if it parses, otherwise return the input unchanged. */
export function prettyJson(value: unknown): string {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  try {
    return JSON.stringify(value, null, 2) ?? '';
  } catch {
    return String(value);
  }
}

export function pickDefined<T extends Record<string, unknown>>(obj: T): Partial<T> {
  return Object.fromEntries(Object.entries(obj).filter(([, v]) => v !== undefined)) as Partial<T>;
}

export function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

export function percentChange(current: number, previous: number): number | null {
  if (!previous) return current ? null : 0;
  return ((current - previous) / previous) * 100;
}
