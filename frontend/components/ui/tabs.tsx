'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

const base =
  'relative -mb-px inline-flex items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-accent';
const on = 'border-accent text-fg';
const off = 'border-transparent text-muted hover:text-fg';

export interface TabItem {
  value: string;
  label: ReactNode;
  count?: number | null;
  hidden?: boolean;
}

/** State-driven tabs. Render the active panel yourself based on `value`. */
export function Tabs({
  items,
  value,
  onValueChange,
  className,
  ariaLabel,
}: {
  items: TabItem[];
  value: string;
  onValueChange: (value: string) => void;
  className?: string;
  ariaLabel?: string;
}) {
  return (
    <div role="tablist" aria-label={ariaLabel} className={cn('flex gap-1 overflow-x-auto border-b border-line', className)}>
      {items
        .filter((i) => !i.hidden)
        .map((item) => (
          <button
            key={item.value}
            role="tab"
            type="button"
            aria-selected={value === item.value}
            className={cn(base, value === item.value ? on : off)}
            onClick={() => onValueChange(item.value)}
          >
            {item.label}
            {item.count !== undefined && item.count !== null && (
              <span className="rounded-full bg-surface-3 px-1.5 text-[11px] tabular-nums text-muted">{item.count}</span>
            )}
          </button>
        ))}
    </div>
  );
}

export interface TabLinkItem {
  href: string;
  label: ReactNode;
  /** Match only the exact pathname (default for the first/index tab). */
  exact?: boolean;
  hidden?: boolean;
}

/** Route-based tabs (sub-pages). */
export function TabLinks({ items, className }: { items: TabLinkItem[]; className?: string }) {
  const pathname = usePathname();
  return (
    <nav className={cn('flex gap-1 overflow-x-auto border-b border-line', className)}>
      {items
        .filter((i) => !i.hidden)
        .map((item) => {
          const active = item.exact ? pathname === item.href : pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link key={item.href} href={item.href} aria-current={active ? 'page' : undefined} className={cn(base, active ? on : off)}>
              {item.label}
            </Link>
          );
        })}
    </nav>
  );
}
