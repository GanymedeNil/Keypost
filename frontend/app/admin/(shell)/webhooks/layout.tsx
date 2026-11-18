'use client';

import type { ReactNode } from 'react';
import { useT } from '@/lib/i18n/provider';
import { PageHeader, TabLinks } from '@/components/kit';

export default function WebhooksLayout({ children }: { children: ReactNode }) {
  const t = useT();
  return (
    <>
      <PageHeader title={t('webhooks.title')} description={t('webhooks.description')} />
      <TabLinks
        className="mb-4"
        items={[
          { href: '/admin/webhooks', label: t('webhooks.tab.endpoints'), exact: true },
          { href: '/admin/webhooks/deliveries', label: t('webhooks.tab.deliveries') },
        ]}
      />
      {children}
    </>
  );
}
