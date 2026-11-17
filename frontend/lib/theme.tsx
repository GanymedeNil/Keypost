'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, type ReactNode } from 'react';
import { usePersistedString } from './persisted';

export type Theme = 'light' | 'dark' | 'system';
export const THEME_KEY = 'df-theme';

/** Inline script (runs before first paint) to avoid a flash of the wrong theme. */
export const themeInitScript = `(function(){try{var t=localStorage.getItem('${THEME_KEY}');var d=t==='dark'||((!t||t==='system')&&window.matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',d);}catch(e){}})();`;

function apply(theme: Theme) {
  const dark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', dark);
}

interface ThemeValue {
  theme: Theme;
  setTheme: (t: Theme) => void;
}

const ThemeContext = createContext<ThemeValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [stored, setStored] = usePersistedString(THEME_KEY, 'system');
  const theme: Theme = stored === 'light' || stored === 'dark' ? stored : 'system';

  const setTheme = useCallback(
    (t: Theme) => {
      setStored(t);
      apply(t);
    },
    [setStored],
  );

  useEffect(() => {
    apply(theme);
    if (theme !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => apply('system');
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [theme]);

  const value = useMemo(() => ({ theme, setTheme }), [theme, setTheme]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}
