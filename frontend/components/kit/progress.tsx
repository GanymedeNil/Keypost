import { cn } from '@/lib/utils';
import { Progress as UiProgress } from '@/components/ui/progress';

const bars = {
  accent: '',
  success: '[&>[data-slot=progress-indicator]]:bg-success',
  danger: '[&>[data-slot=progress-indicator]]:bg-destructive',
  warning: '[&>[data-slot=progress-indicator]]:bg-warning',
} as const;

/** shadcn Progress with a tone and a plain `value` / `max` pair. */
export function Progress({
  value,
  max = 100,
  tone = 'accent',
  className,
  label,
}: {
  value: number;
  max?: number;
  tone?: keyof typeof bars;
  className?: string;
  label?: string;
}) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  return <UiProgress value={pct} aria-label={label} className={cn('h-1.5', bars[tone], className)} />;
}
