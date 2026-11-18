'use client';

import { useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import * as Menu from '@radix-ui/react-dropdown-menu';
import { Check, ChevronDown, FlaskConical, Globe, LogOut, Menu as MenuIcon, Monitor, Moon, Store as StoreIcon, Sun, User as UserIcon } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { mutate as globalMutate } from 'swr';
import { adminApi } from '@/lib/api';
import { LANGS, LANG_LABELS } from '@/lib/i18n';
import { useI18n } from '@/lib/i18n/provider';
import { useTheme, type Theme } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Badge, Button, Switch } from '@/components/kit';
import { DropdownContent, DropdownItemView, DropdownLabel, DropdownSeparator, type ActionItem } from '@/components/kit/dropdown';
import { useAdmin } from './admin-context';
import { Brand, NavList } from './sidebar';

function initials(name: string, email: string) {
  const src = (name || email).trim();
  const parts = src.split(/[\s@._-]+/).filter(Boolean);
  return ((parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')).toUpperCase();
}

function StoreSwitcher() {
  const { t } = useI18n();
  const { stores, storeId, setStoreId } = useAdmin();
  if (stores.length < 2) return null;
  const current = stores.find((s) => s.id === storeId);
  const items: ActionItem[] = [
    { key: 'all', label: t('common.allStores'), active: storeId === null, onSelect: () => setStoreId(null) },
    ...stores.map((s) => ({ key: String(s.id), label: s.name, active: s.id === storeId, onSelect: () => setStoreId(s.id) })),
  ];
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <Button variant="secondary" size="md" aria-label={t('shell.store')} className="max-w-28 gap-2 sm:max-w-48">
          <StoreIcon className="size-3.5 text-muted-foreground" />
          <span className="truncate">{current?.name ?? t('common.allStores')}</span>
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </Button>
      </Menu.Trigger>
      <DropdownContent align="start">
        <DropdownLabel>{t('shell.store')}</DropdownLabel>
        {items.map((it) => (
          <DropdownItemView key={it.key} it={{ ...it, icon: it.active ? <Check /> : <span className="size-4" /> }} />
        ))}
      </DropdownContent>
    </Menu.Root>
  );
}

function TestModeToggle() {
  const { t } = useI18n();
  const { testMode, setTestMode } = useAdmin();
  return (
    <label
      className={cn(
        'flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1 text-sm transition-colors',
        testMode ? 'border-warning/40 bg-warning-soft text-warning-text' : 'border-border bg-card text-muted-foreground hover:bg-muted',
      )}
    >
      <FlaskConical className="size-3.5" />
      <span className="hidden font-medium sm:inline">{t('shell.testMode')}</span>
      <Switch checked={testMode} onCheckedChange={setTestMode} aria-label={t('shell.testMode')} />
    </label>
  );
}

export function LanguageMenu() {
  const { t, lang, setLang } = useI18n();
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('shell.language')}>
          <Globe className="size-4" />
        </Button>
      </Menu.Trigger>
      <DropdownContent>
        <DropdownLabel>{t('shell.language')}</DropdownLabel>
        {LANGS.map((l) => (
          <DropdownItemView key={l} it={{ label: LANG_LABELS[l], active: l === lang, icon: l === lang ? <Check /> : <span className="size-4" />, onSelect: () => setLang(l) }} />
        ))}
      </DropdownContent>
    </Menu.Root>
  );
}

export function ThemeMenu() {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  const opts: { value: Theme; icon: React.ReactNode; label: string }[] = [
    { value: 'light', icon: <Sun />, label: t('shell.theme.light') },
    { value: 'dark', icon: <Moon />, label: t('shell.theme.dark') },
    { value: 'system', icon: <Monitor />, label: t('shell.theme.system') },
  ];
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('shell.theme')}>
          <Sun className="size-4 dark:hidden" />
          <Moon className="hidden size-4 dark:block" />
        </Button>
      </Menu.Trigger>
      <DropdownContent>
        <DropdownLabel>{t('shell.theme')}</DropdownLabel>
        {opts.map((o) => (
          <DropdownItemView key={o.value} it={{ label: o.label, icon: o.icon, active: theme === o.value, onSelect: () => setTheme(o.value) }} />
        ))}
      </DropdownContent>
    </Menu.Root>
  );
}

function UserMenu() {
  const { t, te } = useI18n();
  const { user, role } = useAdmin();
  const router = useRouter();

  async function signOut() {
    try {
      await adminApi.post('/auth/logout', undefined, { redirectOn401: false });
    } catch {
      /* already signed out */
    }
    await globalMutate(() => true, undefined, { revalidate: false });
    router.replace('/admin/login');
  }

  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button
          type="button"
          aria-label={t('shell.userMenu')}
          className="grid size-8 place-items-center rounded-full bg-accent text-xs font-semibold text-primary ring-1 ring-border hover:ring-ring/50"
        >
          {initials(user.name, user.email)}
        </button>
      </Menu.Trigger>
      <DropdownContent className="w-60">
        <div className="px-2.5 py-2">
          <p className="text-xs text-muted-foreground">{t('shell.signedInAs')}</p>
          <p className="truncate text-sm font-medium">{user.name || user.email}</p>
          <p className="truncate text-xs text-muted-foreground">{user.email}</p>
          <Badge tone={role === 'owner' ? 'accent' : 'neutral'} className="mt-1.5">
            {te(`role.${role}`, role)}
          </Badge>
        </div>
        <DropdownSeparator />
        <DropdownItemView it={{ label: t('shell.profile'), icon: <UserIcon />, href: '/admin/settings/profile' }} />
        <DropdownItemView it={{ label: t('shell.signOut'), icon: <LogOut />, onSelect: signOut }} />
      </DropdownContent>
    </Menu.Root>
  );
}

function MobileNav() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>
        <Button variant="ghost" size="icon" className="lg:hidden" aria-label={t('shell.openMenu')}>
          <MenuIcon className="size-4" />
        </Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 animate-fade-in bg-black/50 lg:hidden" />
        <Dialog.Content className="fixed inset-y-0 left-0 z-50 flex w-72 max-w-[85vw] animate-slide-in-left flex-col border-r border-border bg-card shadow-[var(--shadow-pop)] focus:outline-none lg:hidden">
          <Dialog.Title className="sr-only">{t('shell.brand')}</Dialog.Title>
          <Dialog.Description className="sr-only">{t('shell.openMenu')}</Dialog.Description>
          <div className="flex h-14 items-center px-3">
            <Brand />
          </div>
          <NavList onNavigate={() => setOpen(false)} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function Topbar() {
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border bg-card/85 px-3 backdrop-blur sm:px-5">
      <MobileNav />
      <StoreSwitcher />
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <TestModeToggle />
        <LanguageMenu />
        <ThemeMenu />
        <UserMenu />
      </div>
    </header>
  );
}
