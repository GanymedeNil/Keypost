'use client';

import Link from 'next/link';
import { useEffect, type ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
import { cn } from '@/lib/utils';

export function usePageTitle(title: string | undefined) {
  useEffect(() => {
    if (title) document.title = `${title} · Digital Fulfillment`;
  }, [title]);
}

export function PageHeader({
  title,
  description,
  actions,
  back,
  badges,
  className,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  back?: { href: string; label: string };
  /** Badges rendered next to the title. */
  badges?: ReactNode;
  className?: string;
}) {
  usePageTitle(title);
  return (
    <header className={cn('mb-5 flex flex-wrap items-start justify-between gap-x-4 gap-y-3', className)}>
      <div className="min-w-0">
        {back && (
          <Link href={back.href} className="mb-1.5 inline-flex items-center gap-1 text-xs text-subtle hover:text-fg">
            <ArrowLeft className="size-3.5" />
            {back.label}
          </Link>
        )}
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="truncate text-xl font-semibold tracking-tight text-fg">{title}</h1>
          {badges}
        </div>
        {description && <p className="mt-1 max-w-2xl text-sm text-muted">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

/** Definition list: `<DL><DI label="Email">…</DI></DL>`. */
export function DL({ className, children, cols = 1 }: { className?: string; children: ReactNode; cols?: 1 | 2 | 3 }) {
  return (
    <dl className={cn('grid gap-x-6 gap-y-3', cols === 2 && 'sm:grid-cols-2', cols === 3 && 'sm:grid-cols-2 lg:grid-cols-3', className)}>{children}</dl>
  );
}

export function DI({ label, children, className }: { label: ReactNode; children: ReactNode; className?: string }) {
  return (
    <div className={cn('min-w-0', className)}>
      <dt className="text-xs text-subtle">{label}</dt>
      <dd className="mt-0.5 min-w-0 break-words text-sm text-fg">{children === null || children === undefined || children === '' ? <span className="text-subtle">—</span> : children}</dd>
    </div>
  );
}

/** Toolbar row above tables: search + filters on the left, actions on the right. */
export function Toolbar({ children, actions, className }: { children?: ReactNode; actions?: ReactNode; className?: string }) {
  return (
    <div className={cn('flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-3', className)}>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{children}</div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <code className={cn('rounded bg-surface-3 px-1.5 py-0.5 text-xs text-fg', className)}>{children}</code>;
}
