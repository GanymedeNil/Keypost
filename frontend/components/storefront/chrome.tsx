'use client';

import Link from 'next/link';
import { BrandMark } from '@/components/brand-mark';
import { useT } from '@/lib/i18n/provider';
import { LanguageMenu, ThemeMenu } from '@/components/admin/topbar';

export function StorefrontHeader() {
  const t = useT();
  return (
    <header className="border-b border-line bg-surface print:hidden">
      <div className="mx-auto flex h-14 w-full max-w-3xl items-center justify-between gap-3 px-4">
        <Link href="/my-orders" className="flex min-h-10 items-center gap-2.5 rounded-md">
          <span className="grid size-8 place-items-center rounded-lg bg-accent text-accent-fg">
            <BrandMark className="size-5" />
          </span>
          <span className="text-sm font-semibold tracking-tight">{t('storefront.brand')}</span>
        </Link>
        <div className="flex items-center gap-1">
          <LanguageMenu />
          <ThemeMenu />
        </div>
      </div>
    </header>
  );
}

export function StorefrontFooter() {
  const t = useT();
  return (
    <footer className="mx-auto w-full max-w-3xl px-4 pb-8 pt-4 text-center text-xs text-subtle print:hidden">
      {t('storefront.footer')}
    </footer>
  );
}

export function SkipLink() {
  const t = useT();
  return (
    <a
      href="#main"
      className="sr-only z-[200] rounded-md bg-accent px-3 py-1.5 text-accent-fg focus:not-sr-only focus:fixed focus:left-3 focus:top-3"
    >
      {t('storefront.skipToContent')}
    </a>
  );
}
