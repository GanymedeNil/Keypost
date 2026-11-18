'use client';

import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Select } from './field';

export interface FilterOption {
  value: string;
  label: string;
}

/** Compact `<select>` for list filters; the empty value means "all" (`allLabel`). */
export function FilterSelect({
  value,
  onChange,
  options,
  allLabel,
  'aria-label': ariaLabel,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  options: FilterOption[];
  allLabel?: string;
  'aria-label': string;
  className?: string;
}) {
  const t = useT();
  return (
    <div className={cn('w-full sm:w-44', className)}>
      <Select value={value} onChange={(e) => onChange(e.target.value)} aria-label={ariaLabel}>
        <option value="">{allLabel ?? t('common.all')}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </Select>
    </div>
  );
}
