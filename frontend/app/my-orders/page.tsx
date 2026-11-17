'use client';

import { LogOut, ShoppingBag } from 'lucide-react';
import { mutate } from 'swr';
import { storefrontApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { ME_KEY, isAuthError, useMe } from '@/lib/storefront';
import { Button, EmptyState, ErrorState, Skeleton, usePageTitle } from '@/components/ui';
import { OrderCard, SubscriptionCard } from '@/components/storefront/order-list';
import { SignInCard } from '@/components/storefront/sign-in-card';

function ListSkeleton() {
  return (
    <div className="space-y-6" role="status" aria-busy="true">
      <div className="space-y-2">
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <div className="space-y-3">
        {Array.from({ length: 3 }, (_, i) => (
          <Skeleton key={i} className="h-28 w-full rounded-xl" />
        ))}
      </div>
    </div>
  );
}

export default function MyOrdersPage() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate: reload } = useMe();
  const { run, pending } = useAction();
  const signedOut = isAuthError(error);
  usePageTitle(t('storefront.brand'));

  async function signOut() {
    const res = await run(() => storefrontApi.post('/auth/logout'), { success: t('storefront.signedOut') });
    if (res.ok) await mutate(ME_KEY, undefined, { revalidate: true });
  }

  if (signedOut) return <SignInCard />;
  if (error) return <ErrorState error={error} onRetry={() => void reload()} />;
  if (isLoading || !data) return <ListSkeleton />;

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-tight">{t('storefront.me.title')}</h1>
          <p className="mt-1 break-all text-sm text-muted">{t('storefront.me.signedInAs', { email: data.email })}</p>
        </div>
        <Button size="lg" icon={<LogOut className="size-4" />} loading={pending} onClick={() => void signOut()}>
          {t('storefront.signOut')}
        </Button>
      </div>

      <section aria-labelledby="orders-heading" className="space-y-3">
        <h2 id="orders-heading" className="text-sm font-semibold text-muted">
          {t('storefront.me.orders')}
        </h2>
        {data.orders.length === 0 ? (
          <div className="rounded-xl border border-dashed border-line-strong bg-surface">
            <EmptyState
              icon={<ShoppingBag className="size-5" />}
              title={t('storefront.me.noOrders')}
              description={t('storefront.me.noOrdersHint', { email: data.email })}
            />
          </div>
        ) : (
          <ul className="space-y-3">
            {data.orders.map((o) => (
              <OrderCard key={o.identifier} order={o} />
            ))}
          </ul>
        )}
      </section>

      {data.subscriptions.length > 0 && (
        <section aria-labelledby="subs-heading" className="space-y-3">
          <h2 id="subs-heading" className="text-sm font-semibold text-muted">
            {t('storefront.me.subscriptions')}
          </h2>
          <ul className="space-y-3">
            {data.subscriptions.map((s) => (
              <SubscriptionCard key={s.id} sub={s} />
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
