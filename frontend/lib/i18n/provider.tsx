'use client';

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { dictionaries, LANG_COOKIE, LOCALES, type Lang, type MessageKey } from './index';

export type TParams = Record<string, string | number | null | undefined>;
export type TFunction = (key: MessageKey, params?: TParams) => string;

interface I18nValue {
  lang: Lang;
  locale: string;
  setLang: (lang: Lang) => void;
  t: TFunction;
  /** Translate a dynamically built key (e.g. `status.${s}`), falling back to `fallback` when unknown. */
  te: (key: string, fallback: string, params?: TParams) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

function interpolate(template: string, params?: TParams): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (m, name: string) => {
    const v = params[name];
    return v === undefined || v === null ? m : String(v);
  });
}

export function I18nProvider({ initialLang, children }: { initialLang: Lang; children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    document.documentElement.lang = LOCALES[next];
    document.cookie = `${LANG_COOKIE}=${next}; path=/; max-age=31536000; samesite=lax`;
    try {
      localStorage.setItem(LANG_COOKIE, next);
    } catch {
      /* storage unavailable */
    }
  }, []);

  const value = useMemo<I18nValue>(() => {
    const dict = dictionaries[lang];
    const fallback = dictionaries.en;
    const lookup = (key: string) => dict[key] ?? fallback[key];
    return {
      lang,
      locale: LOCALES[lang],
      setLang,
      t: (key, params) => interpolate(lookup(key) ?? key, params),
      te: (key, fb, params) => {
        const v = lookup(key);
        return v === undefined ? fb : interpolate(v, params);
      },
    };
  }, [lang, setLang]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used inside <I18nProvider>');
  return ctx;
}

/** Translation function: `const t = useT(); t('common.save')`. */
export function useT(): TFunction {
  return useI18n().t;
}
