'use client';

import { LicenseForm } from '@/components/admin/settings/license-form';
import { SettingsGate } from '@/components/admin/settings/settings-gate';

export default function LicenseSettingsPage() {
  return <SettingsGate>{(ctx) => <LicenseForm ctx={ctx} />}</SettingsGate>;
}
