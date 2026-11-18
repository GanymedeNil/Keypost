'use client';

import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react';
import { useT } from '@/lib/i18n/provider';
import { Button } from './button';
import { Modal } from './dialog';

export interface ConfirmOptions {
  title: ReactNode;
  description?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** `danger` renders a red confirm button (default for destructive actions). */
  tone?: 'danger' | 'default';
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;
const ConfirmContext = createContext<ConfirmFn | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const t = useT();
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((v: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((o) => {
    return new Promise<boolean>((resolve) => {
      resolver.current?.(false);
      resolver.current = resolve;
      setOpts(o);
    });
  }, []);

  const settle = (value: boolean) => {
    resolver.current?.(value);
    resolver.current = null;
    setOpts(null);
  };

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Modal
        open={opts !== null}
        onOpenChange={(o) => !o && settle(false)}
        title={opts?.title ?? ''}
        description={opts?.description}
        size="sm"
        footer={
          <>
            <Button onClick={() => settle(false)}>{opts?.cancelLabel ?? t('common.cancel')}</Button>
            <Button variant={opts?.tone === 'default' ? 'primary' : 'danger'} onClick={() => settle(true)} autoFocus>
              {opts?.confirmLabel ?? t('common.confirm')}
            </Button>
          </>
        }
      />
    </ConfirmContext.Provider>
  );
}

/** `const confirm = useConfirm(); if (await confirm({ title: '…' })) { … }` */
export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error('useConfirm must be used inside <ConfirmProvider>');
  return ctx;
}

/** Shorthand for the standard "Delete {name}? This cannot be undone." prompt. */
export function useConfirmDelete() {
  const confirm = useConfirm();
  const t = useT();
  return useCallback(
    (name: string, description?: ReactNode) =>
      confirm({
        title: t('confirm.deleteTitle', { name }),
        description: description ?? t('confirm.deleteBody'),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }),
    [confirm, t],
  );
}
