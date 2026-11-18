'use client';

import { CreditCard } from 'lucide-react';
import { storefrontApi } from '@/lib/api';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, ButtonAnchor, useToast, type ButtonVariant } from '@/components/kit';

function BillingPortalButton({ variant, className }: { variant: ButtonVariant; className?: string }) {
  const t = useT();
  const toast = useToast();
  const { run, pending } = useAction();

  async function open() {
    const res = await run(() => storefrontApi.get<{ url: string }>('/billing-url'), { silent: true });
    if (res.ok && res.data.url) window.location.assign(res.data.url);
    else if (!res.ok) toast.error(t('storefront.sub.billingFailed'));
  }

  return (
    <Button variant={variant} size="lg" className={className} loading={pending} icon={<CreditCard className="size-4" />} onClick={() => void open()}>
      {pending ? t('storefront.sub.opening') : t('storefront.sub.manageBilling')}
    </Button>
  );
}

/**
 * "Manage billing": uses the signed `billing_url` from the API as a plain link when present
 * (the backend redirects to the Stripe portal), otherwise asks `/billing-url` for a portal session.
 */
export function ManageBilling({ href, variant = 'secondary', className }: { href?: string | null; variant?: ButtonVariant; className?: string }) {
  const t = useT();
  if (href) {
    return (
      <ButtonAnchor href={href} variant={variant} size="lg" className={className} icon={<CreditCard className="size-4" />}>
        {t('storefront.sub.manageBilling')}
      </ButtonAnchor>
    );
  }
  return <BillingPortalButton variant={variant} className={className} />;
}
