'use client';

import { useT } from '@/lib/i18n/provider';
import { Badge } from '@/components/ui';

export function LivemodeBadge({ livemode }: { livemode: boolean }) {
  const t = useT();
  return <Badge tone={livemode ? 'success' : 'warning'}>{livemode ? t('stripe.live') : t('stripe.testBadge')}</Badge>;
}
