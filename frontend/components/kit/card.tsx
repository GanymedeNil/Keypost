import type { ComponentProps, ReactNode } from 'react';
import { cn } from '@/lib/utils';
import {
  Card as UiCard,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter as UiCardFooter,
  CardHeader as UiCardHeader,
  CardTitle,
} from '@/components/ui/card';

/* shadcn Card, with the paddings tightened for an admin UI (header / body / footer sections). */
export function Card({ className, ...props }: ComponentProps<'section'>) {
  return <UiCard className={cn('min-w-0 gap-0 py-0', className)} {...(props as ComponentProps<'div'>)} />;
}

export function CardHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <UiCardHeader className={cn('grid-rows-[auto] gap-1 border-b px-5 py-4 has-data-[slot=card-description]:grid-rows-[auto_auto]', className)}>
      <CardTitle className="text-sm leading-snug">{title}</CardTitle>
      {description && <CardDescription className="text-xs">{description}</CardDescription>}
      {actions && <CardAction className="flex items-center gap-2 self-center">{actions}</CardAction>}
    </UiCardHeader>
  );
}

export function CardBody({ className, ...props }: ComponentProps<'div'>) {
  return <CardContent className={cn('px-5 py-4', className)} {...props} />;
}

export function CardFooter({ className, ...props }: ComponentProps<'div'>) {
  return <UiCardFooter className={cn('justify-end gap-2 border-t px-5 py-3 [.border-t]:pt-3', className)} {...props} />;
}
