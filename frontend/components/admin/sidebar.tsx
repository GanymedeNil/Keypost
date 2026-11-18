'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { BrandLogo } from '@/components/brand-mark';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { NAV, SETTINGS_ITEM, type NavItem } from './nav';

function isActive(pathname: string, item: NavItem) {
  return item.exact ? pathname === item.href : pathname === item.href || pathname.startsWith(`${item.href}/`);
}

function NavLink({ item, onNavigate }: { item: NavItem; onNavigate?: () => void }) {
  const t = useT();
  const pathname = usePathname();
  const active = isActive(pathname, item);
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'group flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors',
        active ? 'bg-accent-soft text-accent-text' : 'text-muted hover:bg-surface-2 hover:text-fg',
      )}
    >
      <Icon className={cn('size-4 shrink-0', active ? 'text-accent-text' : 'text-subtle group-hover:text-fg')} />
      {t(item.label)}
    </Link>
  );
}

export function Brand() {
  const t = useT();
  return (
    <Link href="/admin" className="flex items-center gap-2.5 px-2.5 py-1">
      <BrandLogo label={t('shell.brand')} className="h-6 w-auto shrink-0 text-fg" />
    </Link>
  );
}

export function NavList({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  return (
    <nav aria-label={t('shell.nav.aria')} className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 py-2">
      {NAV.map((group, i) => (
        <div key={i} className="flex flex-col gap-0.5">
          {group.label && <p className="px-2.5 pb-1 text-[11px] font-medium uppercase tracking-wider text-subtle">{t(group.label)}</p>}
          {group.items.map((item) => (
            <NavLink key={item.href} item={item} onNavigate={onNavigate} />
          ))}
        </div>
      ))}
      <div className="mt-auto border-t border-line pt-3">
        <NavLink item={SETTINGS_ITEM} onNavigate={onNavigate} />
      </div>
    </nav>
  );
}

export function Sidebar() {
  return (
    <aside className="sticky top-0 hidden h-dvh w-60 shrink-0 flex-col border-r border-line bg-surface lg:flex">
      <div className="flex h-14 items-center px-3">
        <Brand />
      </div>
      <NavList />
    </aside>
  );
}
