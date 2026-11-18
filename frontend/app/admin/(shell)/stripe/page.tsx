'use client';

import { useI18n } from '@/lib/i18n/provider';
import { useUrlState } from '@/lib/url-state';
import { PageHeader, Tabs } from '@/components/kit';
import { CatalogTab } from '@/components/admin/stripe/catalog-tab';
import { EventsTab } from '@/components/admin/stripe/events-tab';
import { StatusTab } from '@/components/admin/stripe/status-tab';

const TABS = ['status', 'events', 'catalog'] as const;
type TabId = (typeof TABS)[number];

export default function StripePage() {
  const { t } = useI18n();
  const url = useUrlState();
  const raw = url.get('tab');
  const tab: TabId = (TABS as readonly string[]).includes(raw) ? (raw as TabId) : 'status';

  // Switching tabs drops the other tabs' filters / open drawers from the URL.
  const select = (next: string) => url.set({ tab: next === 'status' ? null : next, q: null, status: null, type: null, event: null });

  return (
    <>
      <PageHeader title={t('stripe.title')} description={t('stripe.description')} />
      <Tabs
        className="mb-4"
        value={tab}
        onValueChange={select}
        ariaLabel={t('stripe.title')}
        items={[
          { value: 'status', label: t('stripe.tab.status') },
          { value: 'events', label: t('stripe.tab.events') },
          { value: 'catalog', label: t('stripe.tab.catalog') },
        ]}
      />
      {tab === 'status' && <StatusTab />}
      {tab === 'events' && <EventsTab />}
      {tab === 'catalog' && <CatalogTab />}
    </>
  );
}
