import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Card } from '@/components/kit';

/** Numbered wizard card. `disabled` greys out and disables every form control inside. */
export function StepCard({
  step,
  title,
  description,
  disabled,
  children,
}: {
  step: number;
  title: string;
  description: ReactNode;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <Card className="overflow-hidden">
      <div className="flex gap-3.5 p-4 sm:p-5">
        <div
          aria-hidden
          className="grid size-7 shrink-0 place-items-center rounded-full bg-accent text-sm font-semibold tabular-nums text-primary"
        >
          {step}
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          <p className="mt-0.5 max-w-3xl text-sm text-muted-foreground">{description}</p>
          <fieldset disabled={disabled} className={cn('m-0 mt-4 min-w-0 border-0 p-0', disabled && 'opacity-60')}>
            {children}
          </fieldset>
        </div>
      </div>
    </Card>
  );
}
