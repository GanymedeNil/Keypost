'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Button } from './button';
import {
  DropdownMenu as UiDropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

/* App-level dropdown helpers on top of shadcn's DropdownMenu. */
export const DropdownMenu = UiDropdownMenu;
export const DropdownTrigger = DropdownMenuTrigger;

export function DropdownContent({ className, align = 'end', ...props }: React.ComponentProps<typeof DropdownMenuContent>) {
  return <DropdownMenuContent align={align} sideOffset={6} collisionPadding={12} className={cn('min-w-44', className)} {...props} />;
}

export function DropdownLabel({ children }: { children: ReactNode }) {
  return <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">{children}</DropdownMenuLabel>;
}

export function DropdownSeparator() {
  return <DropdownMenuSeparator />;
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
  const cls = cn(it.active && 'font-medium');
  const inner = (
    <>
      {it.icon}
      <span className="min-w-0 flex-1 truncate">{it.label}</span>
    </>
  );
  const variant = it.tone === 'danger' ? 'destructive' : 'default';
  if (it.href) {
    return (
      <DropdownMenuItem asChild disabled={it.disabled} variant={variant} className={cls}>
        {it.external ? (
          <a href={it.href} target="_blank" rel="noreferrer">
            {inner}
          </a>
        ) : (
          <Link href={it.href}>{inner}</Link>
        )}
      </DropdownMenuItem>
    );
  }
  return (
    <DropdownMenuItem className={cls} variant={variant} disabled={it.disabled} onSelect={it.onSelect}>
      {inner}
    </DropdownMenuItem>
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
    <UiDropdownMenu>
      <DropdownMenuTrigger asChild>
        {trigger ?? (
          <Button variant="ghost" size="icon-sm" aria-label={t('common.actions')} className={className}>
            <MoreHorizontal className="size-4" />
          </Button>
        )}
      </DropdownMenuTrigger>
      <DropdownContent align={align}>
        {label && <DropdownLabel>{label}</DropdownLabel>}
        {visible.map((it, i) => (
          <div key={it.key ?? i}>
            {it.separatorBefore && i > 0 && <DropdownSeparator />}
            <DropdownItemView it={it} />
          </div>
        ))}
      </DropdownContent>
    </UiDropdownMenu>
  );
}
