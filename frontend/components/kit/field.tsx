'use client';

import { createContext, useContext, useId, type AriaAttributes, type ComponentProps, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Input as UiInput } from '@/components/ui/input';
import { Textarea as UiTextarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';

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
              <Label htmlFor={id}>
                {label}
                {required && <span className="text-destructive" aria-hidden>*</span>}
                {optional && <span className="font-normal text-muted-foreground">({t('common.optional')})</span>}
              </Label>
            )}
            {aside}
          </div>
        )}
        {children}
        {error ? (
          <p id={`${id}-err`} role="alert" className="text-xs text-destructive">
            {error}
          </p>
        ) : hint ? (
          <p id={`${id}-hint`} className="text-xs text-muted-foreground">
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

/* The native <select> and the prefix/suffix input borrow the look of shadcn's Input. */
const control =
  'h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs transition-[color,box-shadow] outline-none ' +
  'placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30 ' +
  'focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 ' +
  'aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40';

export function Input({ className, ...props }: ComponentProps<'input'>) {
  const f = useFieldProps(props);
  return <UiInput {...f} {...props} className={className} />;
}

export function Textarea({ className, rows = 3, ...props }: ComponentProps<'textarea'>) {
  const f = useFieldProps(props);
  return <UiTextarea rows={rows} {...f} {...props} className={cn('min-h-16', className)} />;
}

export function Select({ className, children, ...props }: ComponentProps<'select'>) {
  const f = useFieldProps(props);
  return (
    <div className="relative">
      <select {...f} {...props} className={cn(control, 'appearance-none pr-8', className)}>
        {children}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute right-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  );
}

/** Input with a fixed prefix/suffix label inside (e.g. currency code, "days"). */
export function InputGroup({ prefix, suffix, className, ...props }: ComponentProps<'input'> & { prefix?: ReactNode; suffix?: ReactNode }) {
  const f = useFieldProps(props);
  return (
    <div className="relative">
      {prefix && <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{prefix}</span>}
      <UiInput {...f} {...props} className={cn(prefix ? 'pl-10' : undefined, suffix ? 'pr-14' : undefined, className)} />
      {suffix && <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{suffix}</span>}
    </div>
  );
}
