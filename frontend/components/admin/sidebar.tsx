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
        active
          ? 'bg-sidebar-accent text-sidebar-accent-foreground'
          : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground',
      )}
    >
      <Icon className={cn('size-4 shrink-0', active ? 'text-sidebar-accent-foreground' : 'text-sidebar-foreground/60 group-hover:text-sidebar-accent-foreground')} />
      {t(item.label)}
    </Link>
  );
}

export function Brand() {
  const t = useT();
  return (
    <Link href="/admin" className="flex items-center gap-2.5 px-2.5 py-1">
      <BrandLogo label={t('shell.brand')} className="h-6 w-auto shrink-0 text-foreground" />
    </Link>
  );
}

export function NavList({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  return (
    <nav aria-label={t('shell.nav.aria')} className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 py-2">
      {NAV.map((group, i) => (
        <div key={i} className="flex flex-col gap-0.5">
          {group.label && <p className="px-2.5 pb-1 text-xs font-medium text-sidebar-foreground/50">{t(group.label)}</p>}
          {group.items.map((item) => (
            <NavLink key={item.href} item={item} onNavigate={onNavigate} />
          ))}
        </div>
      ))}
      <div className="mt-auto border-t border-sidebar-border pt-3">
        <NavLink item={SETTINGS_ITEM} onNavigate={onNavigate} />
      </div>
    </nav>
  );
}

export function Sidebar() {
  return (
    <aside className="sticky top-0 hidden h-dvh w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground lg:flex">
      <div className="flex h-14 items-center px-3">
        <Brand />
      </div>
      <NavList />
    </aside>
  );
}
