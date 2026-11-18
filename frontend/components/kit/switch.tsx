'use client';

import { useId, type ReactNode } from 'react';
import { cn } from '@/lib/utils';

export interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  id?: string;
  'aria-label'?: string;
  className?: string;
}

export function Switch({ checked, onCheckedChange, disabled, className, ...props }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent transition-colors',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:opacity-50',
        checked ? 'bg-accent' : 'bg-line-strong',
        className,
      )}
      {...props}
    >
      <span
        aria-hidden
        className={cn('block size-4 rounded-full bg-white shadow transition-transform', checked ? 'translate-x-[18px]' : 'translate-x-0.5')}
      />
    </button>
  );
}

/** Switch with a label and optional description laid out as a row. */
export function SwitchField({
  label,
  description,
  className,
  ...props
}: SwitchProps & { label: ReactNode; description?: ReactNode }) {
  const id = useId();
  return (
    <div className={cn('flex items-start justify-between gap-4', className)}>
      <div className="min-w-0">
        <label htmlFor={id} className="text-sm font-medium text-fg">
          {label}
        </label>
        {description && <p className="mt-0.5 text-xs text-subtle">{description}</p>}
      </div>
      <Switch id={id} {...props} />
    </div>
  );
}

export interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label?: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
  className?: string;
  'aria-label'?: string;
}

export function Checkbox({ checked, onCheckedChange, label, description, disabled, className, ...props }: CheckboxProps) {
  const id = useId();
  return (
    <div className={cn('flex items-start gap-2', className)}>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onCheckedChange(e.target.checked)}
        className="mt-0.5 size-4 shrink-0 cursor-pointer rounded border-line-strong accent-accent disabled:cursor-not-allowed disabled:opacity-50"
        {...props}
      />
      {(label || description) && (
        <label htmlFor={id} className={cn('min-w-0 cursor-pointer text-sm', disabled && 'cursor-not-allowed opacity-60')}>
          {label}
          {description && <span className="block text-xs text-subtle">{description}</span>}
        </label>
      )}
    </div>
  );
}
