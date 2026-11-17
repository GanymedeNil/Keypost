'use client';

import { SmtpForm } from '@/components/admin/settings/email-form';
import { SettingsGate } from '@/components/admin/settings/settings-gate';

export default function EmailSettingsPage() {
  return <SettingsGate>{(ctx) => <SmtpForm ctx={ctx} />}</SettingsGate>;
}
