'use client';

import { useState } from 'react';
import { X } from 'lucide-react';
import { Input } from '@/components/kit';
import { useT } from '@/lib/i18n/provider';

/** Free-text tag editor: Enter / comma / space / blur add entries, Backspace on empty removes the last. */
export function ChipInput({
  values,
  onChange,
  validate,
  placeholder,
  disabled,
}: {
  values: string[];
  onChange: (values: string[]) => void;
  /** Return an error message to reject a token. */
  validate?: (token: string) => string | null;
  placeholder?: string;
  disabled?: boolean;
}) {
  const t = useT();
  const [draft, setDraft] = useState('');
  const [error, setError] = useState<string | null>(null);

  function commit(raw: string) {
    const tokens = raw.split(/[\s,]+/).filter(Boolean);
    const next = [...values];
    for (let i = 0; i < tokens.length; i++) {
      const token = tokens[i];
      const problem = validate?.(token) ?? null;
      if (problem) {
        setError(problem);
        setDraft(tokens.slice(i).join(' '));
        if (next.length !== values.length) onChange(next);
        return;
      }
      if (!next.includes(token)) next.push(token);
    }
    setError(null);
    setDraft('');
    if (next.length !== values.length) onChange(next);
  }

  return (
    <div className="space-y-2">
      <Input
        value={draft}
        disabled={disabled}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={error ? true : undefined}
        className="font-mono"
        onChange={(e) => {
          const v = e.target.value;
          setDraft(v);
          if (/[,\s]/.test(v)) commit(v);
          else if (error) setError(null);
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            if (draft.trim()) commit(draft);
          } else if (e.key === 'Backspace' && !draft && values.length > 0) {
            onChange(values.slice(0, -1));
          }
        }}
        onBlur={() => draft.trim() && commit(draft)}
      />
      {error && (
        <p role="alert" className="text-xs text-danger-text">
          {error}
        </p>
      )}
      {values.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {values.map((v) => (
            <li key={v} className="inline-flex items-center gap-1 rounded-md bg-accent py-0.5 pl-2 pr-1 font-mono text-xs text-foreground">
              {v}
              {!disabled && (
                <button
                  type="button"
                  aria-label={t('settings.stripe.login.accountRemove', { account: v })}
                  onClick={() => onChange(values.filter((x) => x !== v))}
                  className="rounded p-0.5 text-muted-foreground hover:bg-input hover:text-foreground"
                >
                  <X className="size-3" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
