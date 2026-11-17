'use client';

import { useRef, useState } from 'react';
import { Search, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';

/** Debounced search box. `value` is the committed value (e.g. from the URL); `onChange` fires after typing pauses. */
export function SearchInput({
  value,
  onChange,
  placeholder,
  className,
  delay = 350,
  'aria-label': ariaLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  delay?: number;
  'aria-label'?: string;
}) {
  const t = useT();
  const [draft, setDraft] = useState(value);
  const [seen, setSeen] = useState(value);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Adopt external changes (back/forward, "clear filters") during render rather than in an effect.
  if (value !== seen) {
    setSeen(value);
    setDraft(value);
  }

  function commit(v: string) {
    clearTimeout(timer.current);
    setSeen(v);
    onChange(v);
  }

  return (
    <div className={cn('relative w-full sm:w-64', className)}>
      <Search aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
      <input
        type="search"
        value={draft}
        placeholder={placeholder ?? t('common.searchPlaceholder')}
        aria-label={ariaLabel ?? placeholder ?? t('common.search')}
        onChange={(e) => {
          const v = e.target.value;
          setDraft(v);
          clearTimeout(timer.current);
          timer.current = setTimeout(() => commit(v), delay);
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') commit(draft);
          if (e.key === 'Escape' && draft) {
            setDraft('');
            commit('');
          }
        }}
        className="h-8 w-full rounded-md border border-line-strong bg-surface pl-8 pr-7 text-sm placeholder:text-subtle [appearance:none] hover:border-subtle focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 [&::-webkit-search-cancel-button]:hidden"
      />
      {draft && (
        <button
          type="button"
          aria-label={t('common.clearFilters')}
          onClick={() => {
            setDraft('');
            commit('');
          }}
          className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-subtle hover:bg-surface-3 hover:text-fg"
        >
          <X className="size-3.5" />
        </button>
      )}
    </div>
  );
}
