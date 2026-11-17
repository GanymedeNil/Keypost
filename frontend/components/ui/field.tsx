'use client';

import { createContext, useContext, useId, type AriaAttributes, type ComponentProps, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';

interface FieldCtx {
  id: string;
  invalid: boolean;
  describedBy?: string;
}
const FieldContext = createContext<FieldCtx | null>(null);

export interface FieldProps {
  label?: ReactNode;
  hint?: ReactNode;
  error?: string | null;
  required?: boolean;
  optional?: boolean;
  /** Extra element rendered at the right of the label row (e.g. a "generate" link). */
  aside?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** Label + control + hint/error. Input/Select/Textarea inside pick up id & aria attributes automatically. */
export function Field({ label, hint, error, required, optional, aside, className, children }: FieldProps) {
  const t = useT();
  const id = useId();
  const describedBy = error ? `${id}-err` : hint ? `${id}-hint` : undefined;
  return (
    <FieldContext.Provider value={{ id, invalid: !!error, describedBy }}>
      <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
        {(label || aside) && (
          <div className="flex items-center justify-between gap-2">
            {label && (
              <label htmlFor={id} className="text-xs font-medium text-fg">
                {label}
                {required && <span className="ml-0.5 text-danger" aria-hidden>*</span>}
                {optional && <span className="ml-1 font-normal text-subtle">({t('common.optional')})</span>}
              </label>
            )}
            {aside}
          </div>
        )}
        {children}
        {error ? (
          <p id={`${id}-err`} role="alert" className="text-xs text-danger-text">
            {error}
          </p>
        ) : hint ? (
          <p id={`${id}-hint`} className="text-xs text-subtle">
            {hint}
          </p>
        ) : null}
      </div>
    </FieldContext.Provider>
  );
}

function useFieldProps(props: { id?: string; 'aria-invalid'?: AriaAttributes['aria-invalid'] }) {
  const ctx = useContext(FieldContext);
  return {
    id: props.id ?? ctx?.id,
    'aria-invalid': props['aria-invalid'] ?? (ctx?.invalid ? true : undefined),
    'aria-describedby': ctx?.describedBy,
  } as const;
}

const control =
  'w-full rounded-md border border-line-strong bg-surface text-sm text-fg placeholder:text-subtle transition-colors ' +
  'hover:border-subtle focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 ' +
  'disabled:cursor-not-allowed disabled:bg-surface-2 disabled:text-muted aria-[invalid=true]:border-danger aria-[invalid=true]:ring-danger/30 [&:is(input,textarea)[readonly]]:bg-surface-2';

export function Input({ className, ...props }: ComponentProps<'input'>) {
  const f = useFieldProps(props);
  return <input {...f} {...props} className={cn(control, 'h-8 px-2.5', className)} />;
}

export function Textarea({ className, rows = 3, ...props }: ComponentProps<'textarea'>) {
  const f = useFieldProps(props);
  return <textarea rows={rows} {...f} {...props} className={cn(control, 'min-h-16 px-2.5 py-1.5', className)} />;
}

export function Select({ className, children, ...props }: ComponentProps<'select'>) {
  const f = useFieldProps(props);
  return (
    <div className="relative">
      <select {...f} {...props} className={cn(control, 'h-8 appearance-none py-0 pl-2.5 pr-8', className)}>
        {children}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
    </div>
  );
}

/** Input with a fixed prefix/suffix label inside (e.g. currency code, "days"). */
export function InputGroup({ prefix, suffix, className, ...props }: ComponentProps<'input'> & { prefix?: ReactNode; suffix?: ReactNode }) {
  const f = useFieldProps(props);
  return (
    <div className="relative">
      {prefix && <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-xs text-subtle">{prefix}</span>}
      <input
        {...f}
        {...props}
        className={cn(control, 'h-8', prefix ? 'pl-9' : 'pl-2.5', suffix ? 'pr-12' : 'pr-2.5', className)}
      />
      {suffix && <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-xs text-subtle">{suffix}</span>}
    </div>
  );
}
