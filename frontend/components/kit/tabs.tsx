'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Tabs as UiTabs, TabsList, TabsTrigger, tabsListVariants } from '@/components/ui/tabs';

export interface TabItem {
  value: string;
  label: ReactNode;
  count?: number | null;
  hidden?: boolean;
}

/** State-driven tabs (shadcn Tabs, "line" style). Render the active panel yourself based on `value`. */
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
    <UiTabs value={value} onValueChange={onValueChange} className={className}>
      <TabsList variant="line" aria-label={ariaLabel} className="h-auto w-full justify-start overflow-x-auto border-b p-0">
        {items
          .filter((i) => !i.hidden)
          .map((item) => (
            <TabsTrigger key={item.value} value={item.value} className="h-9 flex-none rounded-none px-3 after:bottom-[-1px]">
              {item.label}
              {item.count !== undefined && item.count !== null && (
                <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">{item.count}</span>
              )}
            </TabsTrigger>
          ))}
      </TabsList>
    </UiTabs>
  );
}

export interface TabLinkItem {
  href: string;
  label: ReactNode;
  /** Match only the exact pathname (default for the first/index tab). */
  exact?: boolean;
  hidden?: boolean;
}

const linkBase =
  "relative inline-flex h-9 flex-none items-center justify-center gap-1.5 whitespace-nowrap px-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground " +
  "focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none after:absolute after:inset-x-0 after:bottom-[-1px] after:h-0.5 after:bg-foreground after:opacity-0 after:transition-opacity";

/** Route-based tabs (sub-pages), styled like shadcn's line tabs. */
export function TabLinks({ items, className }: { items: TabLinkItem[]; className?: string }) {
  const pathname = usePathname();
  return (
    <nav className={cn(tabsListVariants({ variant: 'line' }), 'h-auto w-full justify-start gap-1 overflow-x-auto rounded-none border-b p-0', className)}>
      {items
        .filter((i) => !i.hidden)
        .map((item) => {
          const active = item.exact ? pathname === item.href : pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link
              key={item.href}
              href={item.href}
              aria-current={active ? 'page' : undefined}
              className={cn(linkBase, active && 'text-foreground after:opacity-100')}
            >
              {item.label}
            </Link>
          );
        })}
    </nav>
  );
}
