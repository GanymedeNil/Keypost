'use client';

import { Suspense, useEffect, type ReactNode } from 'react';
import { usePathname } from 'next/navigation';
import useSWR from 'swr';
import { FlaskConical } from 'lucide-react';
import { adminApi, fetcher, isApiError } from '@/lib/api';
import type { User } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { AdminProvider, useAdmin } from '@/components/admin/admin-context';
import { Sidebar } from '@/components/admin/sidebar';
import { Topbar } from '@/components/admin/topbar';
import { Button, ButtonLink, ErrorState, PageSkeleton, Spinner } from '@/components/kit';

function TestBanner() {
  const { t } = useI18n();
  const { testMode, setTestMode } = useAdmin();
  if (!testMode) return null;
  return (
    <div role="status" className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-warning/30 bg-warning-soft px-4 py-1.5 text-xs text-warning-text sm:px-6">
      <FlaskConical className="size-3.5 shrink-0" />
      <span className="font-medium">{t('shell.testModeOn')}</span>
      <span className="hidden opacity-80 md:inline">{t('shell.testBanner')}</span>
      <Button variant="ghost" size="sm" className="ml-auto h-6 px-2 text-warning-text hover:bg-warning/15" onClick={() => setTestMode(false)}>
        {t('shell.testBannerOff')}
      </Button>
    </div>
  );
}

function Frame({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  return (
    <div className="flex min-h-dvh">
      <a
        href="#main"
        className="sr-only z-[200] rounded-md bg-primary px-3 py-1.5 text-primary-foreground focus:not-sr-only focus:fixed focus:left-3 focus:top-3"
      >
        {t('shell.skipToContent')}
      </a>
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <TestBanner />
        <main id="main" className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-5 sm:px-6 sm:py-6">
          <Suspense fallback={<PageSkeleton />}>{children}</Suspense>
        </main>
      </div>
    </div>
  );
}

export default function AdminShellLayout({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const { data: user, error } = useSWR<User>(adminApi.url('/me'), fetcher, { revalidateOnFocus: false });
  const pathname = usePathname();
  // MFA is required for every member: a session that has not passed it only gets the MFA page
  const needsMfa = !!user?.mfa?.required && !user.mfa.verified;
  useEffect(() => {
    if (needsMfa) window.location.replace(`/admin/mfa?next=${encodeURIComponent(pathname)}`);
  }, [needsMfa, pathname]);

  if (error && !isApiError(error)) {
    return <ErrorState error={error} title={t('shell.sessionError')} className="min-h-dvh" />;
  }
  if (error) {
    // 401 is redirected to /admin/login by the API client; anything else is surfaced here.
    return (
      <div className="grid min-h-dvh place-items-center">
        <div className="text-center">
          <ErrorState error={error} title={t('shell.sessionError')} />
          <ButtonLink href="/admin/login" variant="primary">
            {t('shell.goToLogin')}
          </ButtonLink>
        </div>
      </div>
    );
  }
  if (!user || needsMfa) {
    return (
      <div className="grid min-h-dvh place-items-center text-sm text-muted-foreground" role="status">
        <div className="flex items-center gap-2">
          <Spinner />
          {t('shell.loadingSession')}
        </div>
      </div>
    );
  }
  return (
    <AdminProvider user={user}>
      <Frame>{children}</Frame>
    </AdminProvider>
  );
}
