'use client';

import { StripeForm } from '@/components/admin/settings/stripe-form';
import { SettingsGate } from '@/components/admin/settings/settings-gate';

export default function StripeSettingsPage() {
  return <SettingsGate>{(ctx) => <StripeForm ctx={ctx} />}</SettingsGate>;
}
