'use client';

import * as Menu from '@radix-ui/react-dropdown-menu';
import Link from 'next/link';
import type { ReactNode } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Button } from './button';

export const DropdownMenu = Menu.Root;
export const DropdownTrigger = Menu.Trigger;

const content =
  'z-[60] min-w-44 animate-fade-in overflow-hidden rounded-lg border border-line bg-surface p-1 text-sm shadow-[var(--shadow-pop)]';
const item =
  'flex w-full cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm outline-none ' +
  'data-[highlighted]:bg-surface-2 data-[disabled]:pointer-events-none data-[disabled]:opacity-50';

export function DropdownContent({ className, align = 'end', ...props }: Menu.DropdownMenuContentProps) {
  return (
    <Menu.Portal>
      <Menu.Content align={align} sideOffset={6} collisionPadding={12} className={cn(content, className)} {...props} />
    </Menu.Portal>
  );
}

export function DropdownLabel({ children }: { children: ReactNode }) {
  return <Menu.Label className="px-2.5 py-1.5 text-xs font-medium text-subtle">{children}</Menu.Label>;
}

export function DropdownSeparator() {
  return <Menu.Separator className="-mx-1 my-1 h-px bg-line" />;
}

export interface ActionItem {
  key?: string;
  label: ReactNode;
  icon?: ReactNode;
  onSelect?: () => void;
  href?: string;
  external?: boolean;
  tone?: 'default' | 'danger';
  disabled?: boolean;
  /** Hide the item entirely (e.g. permission gated). */
  hidden?: boolean;
  separatorBefore?: boolean;
  /** When true the item is rendered as `selected` (checkmark-like emphasis). */
  active?: boolean;
}

export function DropdownItemView({ it }: { it: ActionItem }) {
  const cls = cn(item, it.tone === 'danger' && 'text-danger-text data-[highlighted]:bg-danger-soft', it.active && 'font-medium text-accent-text');
  const inner = (
    <>
      {it.icon && <span className="shrink-0 text-subtle [&_svg]:size-4">{it.icon}</span>}
      <span className="min-w-0 flex-1 truncate">{it.label}</span>
    </>
  );
  if (it.href) {
    return (
      <Menu.Item asChild disabled={it.disabled}>
        {it.external ? (
          <a href={it.href} target="_blank" rel="noreferrer" className={cls}>
            {inner}
          </a>
        ) : (
          <Link href={it.href} className={cls}>
            {inner}
          </Link>
        )}
      </Menu.Item>
    );
  }
  return (
    <Menu.Item className={cls} disabled={it.disabled} onSelect={it.onSelect}>
      {inner}
    </Menu.Item>
  );
}

/** Declarative dropdown: `<ActionMenu items={[…]} />` (defaults to a "…" icon trigger). */
export function ActionMenu({
  items,
  trigger,
  label,
  align,
  className,
}: {
  items: ActionItem[];
  trigger?: ReactNode;
  label?: ReactNode;
  align?: 'start' | 'center' | 'end';
  className?: string;
}) {
  const t = useT();
  const visible = items.filter((i) => !i.hidden);
  if (visible.length === 0) return null;
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        {trigger ?? (
          <Button variant="ghost" size="icon-sm" aria-label={t('common.actions')} className={className}>
            <MoreHorizontal className="size-4" />
          </Button>
        )}
      </Menu.Trigger>
      <DropdownContent align={align}>
        {label && <DropdownLabel>{label}</DropdownLabel>}
        {visible.map((it, i) => (
          <div key={it.key ?? i}>
            {it.separatorBefore && i > 0 && <DropdownSeparator />}
            <DropdownItemView it={it} />
          </div>
        ))}
      </DropdownContent>
    </Menu.Root>
  );
}
