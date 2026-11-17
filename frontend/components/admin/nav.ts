import {
  CreditCard,
  DatabaseZap,
  KeyRound,
  LayoutDashboard,
  Package,
  Repeat,
  Settings,
  ShoppingCart,
  Users,
  Webhook,
  type LucideIcon,
} from 'lucide-react';
import type { MessageKey } from '@/lib/i18n';

export interface NavItem {
  href: string;
  label: MessageKey;
  icon: LucideIcon;
  exact?: boolean;
}
export interface NavGroup {
  label?: MessageKey;
  items: NavItem[];
}

export const NAV: NavGroup[] = [
  { items: [{ href: '/admin', label: 'shell.nav.dashboard', icon: LayoutDashboard, exact: true }] },
  {
    label: 'shell.nav.group.sales',
    items: [
      { href: '/admin/orders', label: 'shell.nav.orders', icon: ShoppingCart },
      { href: '/admin/customers', label: 'shell.nav.customers', icon: Users },
      { href: '/admin/subscriptions', label: 'shell.nav.subscriptions', icon: Repeat },
      { href: '/admin/license-keys', label: 'shell.nav.licenseKeys', icon: KeyRound },
    ],
  },
  { label: 'shell.nav.group.catalog', items: [{ href: '/admin/products', label: 'shell.nav.products', icon: Package }] },
  {
    label: 'shell.nav.group.integrations',
    items: [
      { href: '/admin/webhooks', label: 'shell.nav.webhooks', icon: Webhook },
      { href: '/admin/stripe', label: 'shell.nav.stripe', icon: CreditCard },
      { href: '/admin/imports', label: 'shell.nav.imports', icon: DatabaseZap },
    ],
  },
];

export const SETTINGS_ITEM: NavItem = { href: '/admin/settings', label: 'shell.nav.settings', icon: Settings };
