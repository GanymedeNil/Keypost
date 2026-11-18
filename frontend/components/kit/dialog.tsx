'use client';

import * as Dialog from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Button } from './button';

const sizes = { sm: 'max-w-sm', md: 'max-w-lg', lg: 'max-w-2xl', xl: 'max-w-4xl' } as const;

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

/** Centered modal dialog (Radix Dialog: focus trap, Escape, aria). Wrap forms with `<form>` inside children+footer via `as form`: use ModalForm. */
export function Modal({ open, onOpenChange, title, description, children, footer, size = 'md', dismissible = true, className }: ModalProps) {
  const t = useT();
  return (
    <Dialog.Root open={open} onOpenChange={(o) => (dismissible || o) && onOpenChange(o)}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 grid animate-fade-in place-items-center overflow-y-auto bg-scrim p-4">
          <Dialog.Content
            onInteractOutside={(e) => !dismissible && e.preventDefault()}
            onEscapeKeyDown={(e) => !dismissible && e.preventDefault()}
            className={cn(
              'relative my-auto flex max-h-[calc(100dvh-2rem)] w-full animate-pop-in flex-col rounded-xl border border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none',
              sizes[size],
              className,
            )}
          >
            <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
              <div className="min-w-0">
                <Dialog.Title className="text-base font-semibold text-fg">{title}</Dialog.Title>
                {description ? (
                  <Dialog.Description className="mt-1 text-sm text-muted">{description}</Dialog.Description>
                ) : (
                  <Dialog.Description className="sr-only">{typeof title === 'string' ? title : ''}</Dialog.Description>
                )}
              </div>
              <Dialog.Close asChild>
                <Button variant="ghost" size="icon-sm" aria-label={t('common.close')} className="-mr-2 -mt-1">
                  <X className="size-4" />
                </Button>
              </Dialog.Close>
            </div>
            {children && <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>}
            {footer && <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line px-5 py-3">{footer}</div>}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** Right-hand side sheet for detail panels (payload viewers, log details…). */
export function Drawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  width = 'max-w-xl',
}: Omit<ModalProps, 'size' | 'className' | 'dismissible'> & { width?: string }) {
  const t = useT();
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 animate-fade-in bg-scrim" />
        <Dialog.Content
          className={cn(
            'fixed inset-y-0 right-0 z-50 flex w-full animate-slide-in-right flex-col border-l border-line bg-surface shadow-[var(--shadow-pop)] focus:outline-none',
            width,
          )}
        >
          <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
            <div className="min-w-0">
              <Dialog.Title className="text-base font-semibold text-fg">{title}</Dialog.Title>
              {description ? (
                <Dialog.Description className="mt-1 text-sm text-muted">{description}</Dialog.Description>
              ) : (
                <Dialog.Description className="sr-only">{typeof title === 'string' ? title : ''}</Dialog.Description>
              )}
            </div>
            <Dialog.Close asChild>
              <Button variant="ghost" size="icon-sm" aria-label={t('common.close')} className="-mr-2 -mt-1">
                <X className="size-4" />
              </Button>
            </Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
          {footer && <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line px-5 py-3">{footer}</div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
