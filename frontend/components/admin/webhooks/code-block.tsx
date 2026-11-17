'use client';

import { CopyButton } from '@/components/ui';
import { cn } from '@/lib/utils';

/** Monospace, scrollable text block with a copy button (response bodies, errors). */
export function CodeBlock({ value, maxHeight = 240, className }: { value: string; maxHeight?: number; className?: string }) {
  return (
    <div className={cn('relative rounded-lg border border-line bg-surface-2', className)}>
      <div className="absolute right-1.5 top-1.5 z-10">
        <CopyButton value={value} />
      </div>
      <pre className="overflow-auto whitespace-pre-wrap break-all p-3 pr-10 font-mono text-xs leading-relaxed text-fg" style={{ maxHeight }} tabIndex={0}>
        {value}
      </pre>
    </div>
  );
}
