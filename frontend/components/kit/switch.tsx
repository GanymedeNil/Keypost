'use client';

import { useId, type ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Switch as UiSwitch } from '@/components/ui/switch';
import { Checkbox as UiCheckbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';

export interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  id?: string;
  'aria-label'?: string;
  className?: string;
}

/** shadcn Switch with the app's controlled `checked` / `onCheckedChange` props. */
export function Switch({ checked, onCheckedChange, disabled, className, ...props }: SwitchProps) {
  return <UiSwitch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} className={className} {...props} />;
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
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor={id} className="text-sm">
          {label}
        </Label>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
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

/** shadcn Checkbox with an optional label and description. */
export function Checkbox({ checked, onCheckedChange, label, description, disabled, className, ...props }: CheckboxProps) {
  const id = useId();
  return (
    <div className={cn('flex items-start gap-2', className)}>
      <UiCheckbox id={id} checked={checked} disabled={disabled} onCheckedChange={(v) => onCheckedChange(v === true)} className="mt-0.5" {...props} />
      {(label || description) && (
        <Label htmlFor={id} className={cn('flex-col items-start gap-0 font-normal', disabled && 'opacity-60')}>
          <span className="text-sm">{label}</span>
          {description && <span className="text-xs text-muted-foreground">{description}</span>}
        </Label>
      )}
    </div>
  );
}
