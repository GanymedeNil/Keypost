import type { Metadata, Viewport } from 'next';
import { cookies, headers } from 'next/headers';
import './globals.css';
import { Providers } from '@/components/providers';
import { LANG_COOKIE, LOCALES, langFromAcceptLanguage, parseLang } from '@/lib/i18n';
import { themeInitScript } from '@/lib/theme';

export const metadata: Metadata = {
  description: 'Orders, license keys and downloads.',
  robots: { index: false, follow: false },
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  colorScheme: 'light dark',
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  // Language: cookie wins, otherwise the browser's Accept-Language (zh-* → 简体中文).
  const cookie = parseLang((await cookies()).get(LANG_COOKIE)?.value);
  const lang = cookie ?? langFromAcceptLanguage((await headers()).get('accept-language'));
  return (
    <html lang={LOCALES[lang]} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body>
        <Providers initialLang={lang}>{children}</Providers>
      </body>
    </html>
  );
}
