'use client';

import { useMemo, type ReactNode } from 'react';
import { cn, prettyJson } from '@/lib/utils';
import { CopyButton } from './copy-button';

const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;
const MAX_HIGHLIGHT = 150_000;

function highlight(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let i = 0;
  for (const m of text.matchAll(TOKEN)) {
    const idx = m.index ?? 0;
    if (idx > last) out.push(text.slice(last, idx));
    const [full, str, colon, lit, num] = m;
    let cls = '';
    if (str) cls = colon ? 'text-accent-text' : 'text-success-text';
    else if (lit) cls = 'text-warning-text';
    else if (num) cls = 'text-info-text';
    out.push(
      <span key={i++} className={cls}>
        {str ? str : full}
        {colon && <span className="text-muted">{colon}</span>}
      </span>,
    );
    last = idx + full.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

/** Pretty-printed, syntax-highlighted JSON with a copy button. Accepts an object or a JSON string. */
export function JsonViewer({ value, maxHeight = 420, className }: { value: unknown; maxHeight?: number; className?: string }) {
  const text = useMemo(() => prettyJson(value), [value]);
  const nodes = useMemo(() => (text.length > MAX_HIGHLIGHT ? text : highlight(text)), [text]);
  return (
    <div className={cn('relative rounded-lg border border-line bg-surface-2', className)}>
      <div className="absolute right-1.5 top-1.5 z-10">
        <CopyButton value={text} label="JSON" />
      </div>
      <pre
        className="overflow-auto whitespace-pre-wrap break-all p-3 pr-10 text-xs leading-relaxed text-fg"
        style={{ maxHeight }}
        tabIndex={0}
      >
        {nodes}
      </pre>
    </div>
  );
}
