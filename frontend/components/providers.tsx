'use client';

import type { ReactNode } from 'react';
import { SWRConfig } from 'swr';
import { I18nProvider } from '@/lib/i18n/provider';
import type { Lang } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme';
import { fetcher } from '@/lib/api';
import { ToastProvider } from '@/components/kit/toast';
import { ConfirmProvider } from '@/components/kit/confirm';

export function Providers({ initialLang, children }: { initialLang: Lang; children: ReactNode }) {
  return (
    <I18nProvider initialLang={initialLang}>
      <ThemeProvider>
        <SWRConfig
          value={{
            fetcher,
            shouldRetryOnError: false,
            revalidateOnFocus: true,
            focusThrottleInterval: 15_000,
            dedupingInterval: 2000,
          }}
        >
          <ToastProvider>
            <ConfirmProvider>{children}</ConfirmProvider>
          </ToastProvider>
        </SWRConfig>
      </ThemeProvider>
    </I18nProvider>
  );
}
