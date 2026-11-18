'use client';

import { Suspense } from 'react';
import Link from 'next/link';
import { useParams, useSearchParams } from 'next/navigation';
import useSWR from 'swr';
import { ArrowLeft, Lock, PackageSearch, Printer } from 'lucide-react';
import { ApiError, fetcher, storefrontApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate } from '@/lib/format';
import { isAuthError } from '@/lib/storefront';
import type { StorefrontOrder } from '@/lib/types';
import { Button, ButtonLink, Callout, Card, CardBody, EmptyState, ErrorState, Skeleton, StatusBadge, usePageTitle } from '@/components/kit';
import { ManageBilling } from '@/components/storefront/billing';
import { DownloadsSection, InvoiceSection, LicenseKeysSection, LinksSection, ProductLabel, Section } from '@/components/storefront/order-sections';

const hasAmount = (formatted: string) => /[1-9]/.test(formatted);

function OrderSkeleton() {
  return (
    <div className="space-y-5" role="status" aria-busy="true">
      <Skeleton className="h-5 w-24" />
      <div className="space-y-2">
        <Skeleton className="h-8 w-56" />
        <Skeleton className="h-4 w-72" />
      </div>
      <Skeleton className="h-52 w-full rounded-xl" />
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  );
}

function SummarySection({ order }: { order: StorefrontOrder }) {
  const { t } = useI18n();
  return (
    <Section title={t('storefront.order.summary')}>
      <ul className="divide-y divide-border">
        {order.items.map((item, i) => (
          <li key={i} className="flex items-start justify-between gap-4 px-4 py-3 sm:px-5">
            <div className="min-w-0">
              <ProductLabel product={item.product_name} variant={item.variant_name} />
              {item.quantity > 1 && <p className="text-xs text-muted-foreground">{t('storefront.order.qty', { count: item.quantity })}</p>}
            </div>
            <p className="shrink-0 text-sm tabular-nums">{item.price_formatted}</p>
          </li>
        ))}
      </ul>
      <dl className="space-y-1.5 border-t border-border px-4 py-3 text-sm sm:px-5">
        <div className="flex justify-between gap-4">
          <dt className="text-muted-foreground">{t('storefront.order.subtotal')}</dt>
          <dd className="tabular-nums">{order.subtotal_formatted}</dd>
        </div>
        {hasAmount(order.discount_total_formatted) && (
          <div className="flex justify-between gap-4">
            <dt className="text-muted-foreground">{t('storefront.order.discount')}</dt>
            <dd className="tabular-nums">−{order.discount_total_formatted.replace(/^-/, '')}</dd>
          </div>
        )}
        {hasAmount(order.tax_formatted) && (
          <div className="flex justify-between gap-4">
            <dt className="text-muted-foreground">{t('storefront.order.tax')}</dt>
            <dd className="tabular-nums">{order.tax_formatted}</dd>
          </div>
        )}
        <div className="flex justify-between gap-4 border-t border-border pt-2 text-base font-semibold">
          <dt>{t('storefront.order.total')}</dt>
          <dd className="tabular-nums">{order.total_formatted}</dd>
        </div>
      </dl>
      {order.card_last_four && (
        <p className="border-t border-border px-4 py-3 text-xs text-muted-foreground sm:px-5">
          {t('storefront.order.paidWith', { brand: order.card_brand ? order.card_brand.replace(/^./, (c) => c.toUpperCase()) : '', last4: order.card_last_four })}
        </p>
      )}
    </Section>
  );
}

function SubscriptionsSection({ order }: { order: StorefrontOrder }) {
  const { t, locale } = useI18n();
  return (
    <Section title={t('storefront.order.subscriptions')}>
      <ul className="divide-y divide-border">
        {order.subscriptions.map((s) => {
          const ended = s.status === 'cancelled' || s.status === 'expired';
          const note =
            s.ends_at && (ended || !s.renews_at)
              ? t('storefront.sub.endsOn', { date: formatDate(s.ends_at, locale, 'date') })
              : s.renews_at
                ? t('storefront.sub.renewsOn', { date: formatDate(s.renews_at, locale, 'date') })
                : null;
          return (
            <li key={s.id} className="flex flex-wrap items-start justify-between gap-2 px-4 py-3 sm:px-5">
              <div className="min-w-0">
                <p className="text-sm font-medium">{s.product_name}</p>
                {note && <p className="text-xs text-muted-foreground">{note}</p>}
              </div>
              <StatusBadge status={s.status} />
            </li>
          );
        })}
      </ul>
      <div className="border-t border-border p-4 sm:px-5 print:hidden">
        <ManageBilling href={order.billing_url} className="h-11 w-full sm:w-auto" />
      </div>
    </Section>
  );
}

function OrderView({ order }: { order: StorefrontOrder }) {
  const { t, locale } = useI18n();
  usePageTitle(t('storefront.order.title', { number: order.order_number }));
  const keysAllowed = order.license_keys.length > 0;
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3 print:hidden">
        <Link href="/my-orders" className="inline-flex min-h-10 items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" aria-hidden />
          {t('storefront.order.back')}
        </Link>
        <Button variant="ghost" icon={<Printer className="size-4" />} onClick={() => window.print()}>
          {t('storefront.order.print')}
        </Button>
      </div>

      <header className="space-y-1.5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
          <h1 className="text-2xl font-semibold tracking-tight">{t('storefront.order.title', { number: order.order_number })}</h1>
          <StatusBadge status={order.status} />
        </div>
        <p className="text-sm text-muted-foreground">{t('storefront.order.placedOn', { date: formatDate(order.created_at, locale, 'datetime') })}</p>
        {(order.user_name || order.user_email) && (
          <p className="break-all text-xs text-muted-foreground">
            {t('storefront.order.billedTo')}: {[order.user_name, order.user_email].filter(Boolean).join(' · ')}
          </p>
        )}
      </header>

      {order.refunded && <Callout tone="warning">{t('storefront.order.refundedNotice')}</Callout>}

      <SummarySection order={order} />
      {keysAllowed && <LicenseKeysSection keys={order.license_keys} />}
      {order.files.length > 0 && <DownloadsSection files={order.files} />}
      {order.links.length > 0 && <LinksSection links={order.links} />}
      {order.subscriptions.length > 0 && <SubscriptionsSection order={order} />}
      {order.invoice_url && <InvoiceSection url={order.invoice_url} />}
    </div>
  );
}

function OrderPageInner() {
  const { t } = useI18n();
  const { identifier } = useParams<{ identifier: string }>();
  const signature = useSearchParams().get('signature');
  usePageTitle(t('storefront.brand'));

  const key = storefrontApi.url(`/orders/${encodeURIComponent(identifier)}`, { signature });
  const { data, error, isLoading, mutate } = useSWR<StorefrontOrder, ApiError>(key, fetcher, {
    shouldRetryOnError: false,
    revalidateOnFocus: false,
  });

  if (isAuthError(error)) {
    return (
      <Card className="mx-auto max-w-md">
        <CardBody className="flex flex-col items-center gap-3 px-6 py-10 text-center">
          <span className="grid size-12 place-items-center rounded-full bg-accent text-primary">
            <Lock className="size-6" aria-hidden />
          </span>
          <h1 className="text-lg font-semibold tracking-tight">{t('storefront.order.signInRequired.title')}</h1>
          <p className="text-sm text-muted-foreground">{signature ? t('storefront.order.signInRequired.linkInvalid') : t('storefront.order.signInRequired.body')}</p>
          <ButtonLink href="/my-orders" variant="primary" size="lg" className="mt-2 h-11 w-full sm:w-auto">
            {t('storefront.order.signIn')}
          </ButtonLink>
        </CardBody>
      </Card>
    );
  }
  if (error instanceof ApiError && error.status === 404) {
    return (
      <Card className="mx-auto max-w-md">
        <EmptyState
          icon={<PackageSearch className="size-5" />}
          title={t('storefront.order.notFound.title')}
          description={t('storefront.order.notFound.body')}
          action={
            <ButtonLink href="/my-orders" size="lg" className="h-11">
              {t('storefront.order.back')}
            </ButtonLink>
          }
        />
      </Card>
    );
  }
  if (error) return <ErrorState error={error} onRetry={() => void mutate()} />;
  if (isLoading || !data) return <OrderSkeleton />;
  return <OrderView order={data} />;
}

export default function OrderPage() {
  return (
    <Suspense fallback={<OrderSkeleton />}>
      <OrderPageInner />
    </Suspense>
  );
}
