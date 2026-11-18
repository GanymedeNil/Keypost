'use client';

import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { Toaster, toast } from 'sonner';
import { AlertCircle, CheckCircle2, Info } from 'lucide-react';
import { useTheme } from '@/lib/theme';

interface ToastApi {
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

/** Toasts are shadcn's Sonner, themed with the same tokens as the cards and popovers. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const api = useMemo<ToastApi>(
    () => ({
      success: (m) => void toast.success(m, { duration: 4000 }),
      error: (m) => void toast.error(m, { duration: 7000 }),
      info: (m) => void toast.info(m, { duration: 4000 }),
    }),
    [],
  );
  return (
    <ToastContext.Provider value={api}>
      {children}
      <Toaster
        theme={theme}
        position="bottom-right"
        visibleToasts={4}
        icons={{
          success: <CheckCircle2 className="size-4 text-success" />,
          error: <AlertCircle className="size-4 text-destructive" />,
          info: <Info className="size-4 text-info" />,
        }}
        style={
          {
            '--normal-bg': 'var(--popover)',
            '--normal-text': 'var(--popover-foreground)',
            '--normal-border': 'var(--border)',
            '--border-radius': 'var(--radius)',
          } as React.CSSProperties
        }
      />
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used inside <ToastProvider>');
  return ctx;
}
