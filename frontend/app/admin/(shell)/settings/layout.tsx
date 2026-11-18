'use client';

import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { Lock } from 'lucide-react';
import type { MessageKey } from '@/lib/i18n';
import { useT } from '@/lib/i18n/provider';
import { EmptyState, PageHeader, TabLinks } from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';

type Access = 'all' | 'write' | 'owner';

const TABS: { href: string; label: MessageKey; access: Access; exact?: boolean }[] = [
  { href: '/admin/settings', label: 'settings.tabs.general', access: 'write', exact: true },
  { href: '/admin/settings/stripe', label: 'settings.tabs.stripe', access: 'write' },
  { href: '/admin/settings/email', label: 'settings.tabs.email', access: 'write' },
  { href: '/admin/settings/license', label: 'settings.tabs.license', access: 'write' },
  { href: '/admin/settings/api-keys', label: 'settings.tabs.apiKeys', access: 'write' },
  { href: '/admin/settings/team', label: 'settings.tabs.team', access: 'owner' },
  { href: '/admin/settings/audit-log', label: 'settings.tabs.auditLog', access: 'all' },
  { href: '/admin/settings/email-log', label: 'settings.tabs.emailLog', access: 'all' },
  { href: '/admin/settings/profile', label: 'settings.tabs.profile', access: 'all' },
];

export default function SettingsLayout({ children }: { children: ReactNode }) {
  const t = useT();
  const can = useCan();
  const pathname = usePathname();

  const allowed = (access: Access) => access === 'all' || (access === 'write' && can.write) || (access === 'owner' && can.owner);
  const current = TABS.find((tab) => (tab.exact ? pathname === tab.href : pathname === tab.href || pathname.startsWith(`${tab.href}/`)));
  const permitted = !current || allowed(current.access);

  return (
    <>
      <PageHeader title={t('settings.title')} description={t('settings.description')} />
      <TabLinks
        className="mb-6"
        items={TABS.filter((tab) => allowed(tab.access)).map((tab) => ({ href: tab.href, label: t(tab.label), exact: tab.exact }))}
      />
      <div className="max-w-4xl">
        {permitted ? (
          children
        ) : (
          <EmptyState icon={<Lock className="size-5" />} title={t('settings.forbidden.title')} description={t('settings.forbidden.body')} />
        )}
      </div>
    </>
  );
}
