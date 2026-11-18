'use client';

import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Button } from './button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet';

const sizes = { sm: 'sm:max-w-sm', md: 'sm:max-w-lg', lg: 'sm:max-w-2xl', xl: 'sm:max-w-4xl' } as const;

export interface ModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  /** Buttons rendered in a footer row (right aligned). */
  footer?: ReactNode;
  size?: keyof typeof sizes;
  /** Prevent closing via overlay click / Escape (e.g. while submitting). */
  dismissible?: boolean;
  className?: string;
}

function CloseButton() {
  const t = useT();
  return (
    <DialogClose asChild>
      <Button variant="ghost" size="icon-sm" aria-label={t('common.close')} className="-mr-2 -mt-1 shrink-0">
        <X className="size-4" />
      </Button>
    </DialogClose>
  );
}

/** Centered modal dialog: shadcn Dialog (Radix: focus trap, Escape, aria) with a header, a scrolling body and a footer. */
export function Modal({ open, onOpenChange, title, description, children, footer, size = 'md', dismissible = true, className }: ModalProps) {
  return (
    <Dialog open={open} onOpenChange={(o) => (dismissible || o) && onOpenChange(o)}>
      <DialogContent
        showCloseButton={false}
        onInteractOutside={(e) => !dismissible && e.preventDefault()}
        onEscapeKeyDown={(e) => !dismissible && e.preventDefault()}
        className={cn('max-h-[calc(100dvh-2rem)] gap-0 p-0', sizes[size], className)}
      >
        <DialogHeader className="flex-row items-start justify-between gap-4 border-b px-5 py-4 text-left">
          <div className="min-w-0 space-y-1">
            <DialogTitle className="text-base">{title}</DialogTitle>
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : (
              <DialogDescription className="sr-only">{typeof title === 'string' ? title : ''}</DialogDescription>
            )}
          </div>
          <CloseButton />
        </DialogHeader>
        {children && <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>}
        {footer && <DialogFooter className="flex-row flex-wrap items-center justify-end gap-2 border-t px-5 py-3">{footer}</DialogFooter>}
      </DialogContent>
    </Dialog>
  );
}

/** Right-hand side sheet for detail panels (payload viewers, log details…): shadcn Sheet. */
export function Drawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  width = 'sm:max-w-xl',
}: Omit<ModalProps, 'size' | 'className' | 'dismissible'> & { width?: string }) {
  const t = useT();
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent showCloseButton={false} className={cn('w-full gap-0 sm:w-full', width)}>
        <SheetHeader className="flex-row items-start justify-between gap-4 border-b px-5 py-4 text-left">
          <div className="min-w-0 space-y-1">
            <SheetTitle className="text-base">{title}</SheetTitle>
            {description ? <SheetDescription>{description}</SheetDescription> : <SheetDescription className="sr-only">{typeof title === 'string' ? title : ''}</SheetDescription>}
          </div>
          <SheetClose asChild>
            <Button variant="ghost" size="icon-sm" aria-label={t('common.close')} className="-mr-2 -mt-1 shrink-0">
              <X className="size-4" />
            </Button>
          </SheetClose>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
        {footer && <SheetFooter className="flex-row flex-wrap items-center justify-end gap-2 border-t px-5 py-3">{footer}</SheetFooter>}
      </SheetContent>
    </Sheet>
  );
}
