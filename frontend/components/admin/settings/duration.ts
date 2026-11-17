const UNITS: readonly [Intl.NumberFormatOptions['unit'], number][] = [
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
];

/** 3600 -> "1 hour", 5400 -> "90 minutes" (locale aware, largest unit that divides evenly). */
export function humanDuration(seconds: number, locale: string): string {
  const fmt = (n: number, unit: Intl.NumberFormatOptions['unit']) =>
    new Intl.NumberFormat(locale, { style: 'unit', unit, unitDisplay: 'long' }).format(n);
  if (!Number.isFinite(seconds) || seconds <= 0) return fmt(0, 'second');
  for (const [unit, size] of UNITS) {
    if (seconds >= size && seconds % size === 0) return fmt(seconds / size, unit);
  }
  return fmt(seconds, 'second');
}
