import type { ReactNode } from 'react';
import { SkipLink, StorefrontFooter, StorefrontHeader } from '@/components/storefront/chrome';

export default function MyOrdersLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SkipLink />
      <StorefrontHeader />
      <main id="main" className="mx-auto w-full max-w-3xl flex-1 px-4 py-6 sm:py-10 print:max-w-none print:py-0">
        {children}
      </main>
      <StorefrontFooter />
    </div>
  );
}
