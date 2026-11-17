import { cn } from '@/lib/utils';

export function Progress({
  value,
  max = 100,
  tone = 'accent',
  className,
  label,
}: {
  value: number;
  max?: number;
  tone?: 'accent' | 'success' | 'danger' | 'warning';
  className?: string;
  label?: string;
}) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const bar = { accent: 'bg-accent', success: 'bg-success', danger: 'bg-danger', warning: 'bg-warning' }[tone];
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={Math.round(value)}
      className={cn('h-1.5 w-full overflow-hidden rounded-full bg-surface-3', className)}
    >
      <div className={cn('h-full rounded-full transition-[width] duration-300', bar)} style={{ width: `${pct}%` }} />
    </div>
  );
}
