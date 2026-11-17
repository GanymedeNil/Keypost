'use client';

import { useEffect, useState } from 'react';
import { useI18n } from '@/lib/i18n/provider';
import { useUrlState } from '@/lib/url-state';
import { Callout } from '@/components/ui';
import { PasswordCard, ProfileCard, StripeAccountCard } from '@/components/admin/settings/profile-cards';

const STRIPE_ERRORS = ['stripe_denied', 'stripe_state', 'stripe_not_allowed', 'stripe_exchange', 'disabled'];

export default function ProfileSettingsPage() {
  const { t, te } = useI18n();
  const { params, set } = useUrlState();
  // Capture the one-shot flags from the Stripe callback, then strip them from the URL.
  const [linked] = useState(() => params.get('linked') === '1');
  const [errorCode] = useState(() => params.get('error'));
  const hasFlags = params.has('linked') || params.has('error');

  useEffect(() => {
    if (hasFlags) set({ linked: null, error: null });
  }, [hasFlags, set]);

  return (
    <div className="space-y-6">
      {linked && <Callout tone="success">{t('settings.profile.stripe.linkedOk')}</Callout>}
      {errorCode && (
        <Callout tone="danger">{STRIPE_ERRORS.includes(errorCode) ? te(`auth.error.${errorCode}`, t('auth.error.unknown')) : t('auth.error.unknown')}</Callout>
      )}
      <ProfileCard />
      <PasswordCard />
      <StripeAccountCard />
    </div>
  );
}
