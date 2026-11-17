'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState, type ReactNode } from 'react';
import { Download, ExternalLink, Receipt, RotateCcw, Send, User } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { formatDate, formatMoney } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { OrderDetail } from '@/lib/types';
import { useApi } from '@/lib/use-api';
import { OrderTimeline } from '@/components/admin/orders/order-timeline';
import { RefundDialog } from '@/components/admin/orders/refund-dialog';
import { ResendReceiptDialog } from '@/components/admin/orders/resend-dialog';
import { useCan } from '@/components/admin/admin-context';
import {
  Badge,
  Button,
  ButtonAnchor,
  ButtonLink,
  Card,
  CardBody,
  CardHeader,
  CopyButton,
  DI,
  DL,
  DateText,
  ErrorState,
  JsonViewer,
  MoneyText,
  PageHeader,
  PageSkeleton,
  StatusBadge,
  TD,
  TH,
  THead,
  TBody,
  TR,
  Table,
  TableWrap,
  TestModeBadge,
} from '@/components/ui';

function Empty({ children }: { children: ReactNode }) {
  return <p className="px-4 py-8 text-center text-sm text-subtle">{children}</p>;
}

function TotalRow({ label, children, strong, tone }: { label: ReactNode; children: ReactNode; strong?: boolean; tone?: 'danger' }) {
  return (
    <div className={`flex items-baseline justify-between gap-4 py-1 ${strong ? 'border-t border-line pt-2 text-base font-semibold' : 'text-sm'} ${tone === 'danger' ? 'text-danger-text' : ''}`}>
      <dt className={strong ? '' : 'text-muted'}>{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}

function StripeRef({ label, value }: { label: string; value: string | null }) {
  if (!value) return null;
  return (
    <div className="min-w-0">
      <dt className="text-xs text-subtle">{label}</dt>
      <dd className="mt-0.5 flex items-center gap-1">
        <code className="min-w-0 truncate text-xs" title={value}>
          {value}
        </code>
        <CopyButton value={value} label={label} />
      </dd>
    </div>
  );
}

export default function OrderDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t, te, locale } = useI18n();
  const can = useCan();
  const { data: order, error, isLoading, mutate } = useApi<OrderDetail>(`/orders/${id}`);
  const [dialog, setDialog] = useState<'refund' | 'resend' | null>(null);

  if (error && !order) {
    return (
      <>
        <PageHeader title={t('orders.title')} back={{ href: '/admin/orders', label: t('orders.detail.back') }} />
        <Card>
          <ErrorState error={error} onRetry={() => void mutate()} />
        </Card>
      </>
    );
  }
  if (isLoading || !order) return <PageSkeleton />;

  const money = (cents: number) => formatMoney(cents, order.currency, locale);
  const remaining = Math.max(0, order.total - order.refunded_amount);
  const refundable = (order.status === 'paid' || order.status === 'partial_refund') && remaining > 0;
  const taxRate = order.tax_rate !== null && order.tax_rate !== '' ? `${order.tax_rate}%` : null;
  const hasCustomData =
    order.custom_data !== null &&
    order.custom_data !== undefined &&
    !(typeof order.custom_data === 'object' && Object.keys(order.custom_data as object).length === 0);
  const location = [order.city, order.postal_code, order.country].filter(Boolean).join(', ');
  const stripeRefs = [order.stripe_checkout_session, order.stripe_payment_intent, order.stripe_invoice_id, order.stripe_customer_id].some(Boolean);

  return (
    <>
      <PageHeader
        title={t('orders.detail.title', { number: order.order_number })}
        description={
          <>
            {order.user_name ? `${order.user_name} · ` : ''}
            {order.user_email} · <DateText value={order.created_at} />
          </>
        }
        back={{ href: '/admin/orders', label: t('orders.detail.back') }}
        badges={
          <>
            <StatusBadge status={order.status} />
            {order.test_mode && <TestModeBadge />}
          </>
        }
        actions={
          <>
            {can.write && refundable && (
              <Button variant="danger-outline" icon={<RotateCcw className="size-3.5" />} onClick={() => setDialog('refund')}>
                {t('orders.action.refund')}
              </Button>
            )}
            {can.write && (
              <Button icon={<Send className="size-3.5" />} onClick={() => setDialog('resend')}>
                {t('orders.action.resend')}
              </Button>
            )}
            {order.receipt_url && (
              <ButtonAnchor href={order.receipt_url} target="_blank" rel="noreferrer" icon={<Receipt className="size-3.5" />}>
                {t('orders.action.openReceipt')}
                <ExternalLink className="size-3 text-subtle" />
              </ButtonAnchor>
            )}
            {order.customer_id ? (
              <ButtonLink href={`/admin/customers/${order.customer_id}`} icon={<User className="size-3.5" />}>
                {t('orders.action.viewCustomer')}
              </ButtonLink>
            ) : null}
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="min-w-0 space-y-4 lg:col-span-2">
          <Card>
            <CardHeader title={t('orders.section.items')} />
            {order.items.length === 0 ? (
              <Empty>{t('orders.items.empty')}</Empty>
            ) : (
              <TableWrap>
                <Table>
                  <THead>
                    <TR>
                      <TH>{t('common.product')}</TH>
                      <TH className="text-right">{t('orders.items.qty')}</TH>
                      <TH className="hidden text-right sm:table-cell">{t('orders.items.unitPrice')}</TH>
                      <TH className="text-right">{t('orders.items.lineTotal')}</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {order.items.map((item) => (
                      <TR key={item.id}>
                        <TD>
                          <p className="font-medium">{item.product_name}</p>
                          <p className="text-xs text-subtle">{item.variant_name}</p>
                        </TD>
                        <TD className="text-right tabular-nums">{item.quantity}</TD>
                        <TD className="hidden text-right sm:table-cell">
                          <MoneyText amount={item.price} currency={order.currency} />
                        </TD>
                        <TD className="text-right font-medium">
                          <MoneyText amount={item.price * item.quantity} currency={order.currency} />
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableWrap>
            )}
          </Card>

          <Card>
            <CardHeader title={t('orders.section.licenseKeys')} />
            {order.license_keys.length === 0 ? (
              <Empty>{t('orders.keys.empty')}</Empty>
            ) : (
              <ul className="divide-y divide-line">
                {order.license_keys.map((k) => (
                  <li key={k.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1">
                        <code className="min-w-0 truncate rounded bg-surface-3 px-1.5 py-0.5 text-xs" title={k.key}>
                          {k.key}
                        </code>
                        <CopyButton value={k.key} label={t('orders.section.licenseKeys')} />
                      </div>
                      <p className="mt-1 truncate text-xs text-subtle">
                        {k.product_name} — {k.variant_name}
                        {' · '}
                        {t('orders.keys.activations', { used: k.instances_count, limit: k.activation_limit ?? '∞' })}
                        {k.expires_at ? ` · ${t('orders.keys.expires', { date: formatDate(k.expires_at, locale, 'date') })}` : ''}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <StatusBadge status={k.status} />
                      <ButtonLink href={`/admin/license-keys/${k.id}`} size="sm" variant="ghost">
                        {t('common.view')}
                      </ButtonLink>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card>
            <CardHeader title={t('orders.section.files')} />
            {order.files.length === 0 ? (
              <Empty>{t('orders.files.empty')}</Empty>
            ) : (
              <ul className="divide-y divide-line">
                {order.files.map((f) => (
                  <li key={f.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium" title={f.name}>
                        {f.name}
                        {f.extension && !f.name.toLowerCase().endsWith(`.${f.extension.toLowerCase()}`) ? `.${f.extension}` : ''}
                      </p>
                      <p className="text-xs text-subtle">
                        {f.product_name} — {f.variant_name} · {f.size_formatted}
                        {f.version ? ` · v${f.version}` : ''}
                      </p>
                    </div>
                    <ButtonAnchor href={adminApi.url(`/files/${f.id}/download`)} size="sm" icon={<Download className="size-3.5" />}>
                      {t('orders.files.download')}
                    </ButtonAnchor>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card>
            <CardHeader title={t('orders.section.subscriptions')} />
            {order.subscriptions.length === 0 ? (
              <Empty>{t('orders.subs.empty')}</Empty>
            ) : (
              <ul className="divide-y divide-line">
                {order.subscriptions.map((s) => (
                  <li key={s.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3">
                    <div className="min-w-0">
                      <Link href={`/admin/subscriptions/${s.id}`} className="truncate text-sm font-medium hover:text-accent-text">
                        {s.product_name} — {s.variant_name}
                      </Link>
                      <p className="text-xs text-subtle">
                        {s.renews_at
                          ? t('orders.subs.renews', { date: formatDate(s.renews_at, locale, 'date') })
                          : s.ends_at
                            ? t('orders.subs.ends', { date: formatDate(s.ends_at, locale, 'date') })
                            : '—'}
                      </p>
                    </div>
                    <StatusBadge status={s.status} />
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card>
            <CardHeader title={t('orders.section.timeline')} />
            <OrderTimeline emails={order.emails ?? []} deliveries={order.webhook_deliveries ?? []} />
          </Card>
        </div>

        <div className="min-w-0 space-y-4">
          <Card>
            <CardHeader title={t('orders.section.payment')} />
            <CardBody>
              <dl>
                <TotalRow label={t('orders.totals.subtotal')}>{money(order.subtotal)}</TotalRow>
                {order.setup_fee > 0 && <TotalRow label={t('orders.totals.setupFee')}>{money(order.setup_fee)}</TotalRow>}
                {order.discount_total > 0 && <TotalRow label={t('orders.totals.discount')}>−{money(order.discount_total)}</TotalRow>}
                <TotalRow
                  label={
                    <>
                      {order.tax_name || t('orders.totals.tax')}
                      {taxRate ? ` (${taxRate})` : ''}
                      {order.tax_inclusive ? ` · ${t('orders.totals.taxInclusive')}` : ''}
                    </>
                  }
                >
                  {money(order.tax)}
                </TotalRow>
                <TotalRow label={t('orders.totals.total')} strong>
                  {money(order.total)}
                </TotalRow>
                {order.refunded_amount > 0 && (
                  <>
                    <TotalRow label={t('orders.totals.refunded')} tone="danger">
                      −{money(order.refunded_amount)}
                    </TotalRow>
                    <TotalRow label={t('orders.totals.net')}>{money(order.total - order.refunded_amount)}</TotalRow>
                  </>
                )}
              </dl>
              {(order.card_brand || order.card_last_four) && (
                <p className="mt-3 border-t border-line pt-3 text-sm text-muted">
                  <span className="text-xs text-subtle">{t('orders.field.card')}</span>
                  <br />
                  <span className="capitalize">{order.card_brand}</span> •••• {order.card_last_four}
                </p>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader title={t('orders.section.summary')} />
            <CardBody>
              <DL>
                <DI label={t('orders.field.customer')}>
                  {order.customer_id ? (
                    <Link href={`/admin/customers/${order.customer_id}`} className="hover:text-accent-text">
                      {order.user_name || order.user_email}
                    </Link>
                  ) : (
                    order.user_name
                  )}
                  <p className="break-all text-xs text-subtle">{order.user_email}</p>
                </DI>
                <DI label={t('orders.field.identifier')}>
                  <span className="flex items-center gap-1">
                    <code className="min-w-0 truncate text-xs" title={order.identifier}>
                      {order.identifier}
                    </code>
                    <CopyButton value={order.identifier} label={t('orders.field.identifier')} />
                  </span>
                </DI>
                <DI label={t('common.status')}>
                  <StatusBadge status={order.status} />
                </DI>
                <DI label={t('orders.field.source')}>{te(`source.${order.source}`, order.source)}</DI>
                <DI label={t('orders.field.currency')}>{order.currency.toUpperCase()}</DI>
                {location && <DI label={t('orders.field.location')}>{location}</DI>}
                <DI label={t('common.created')}>
                  <DateText value={order.created_at} />
                </DI>
                {order.refunded_at && (
                  <DI label={t('orders.field.refundedAt')}>
                    <DateText value={order.refunded_at} />
                  </DI>
                )}
                {order.affiliate_id ? <DI label={t('orders.field.affiliate')}>#{order.affiliate_id}</DI> : null}
                {order.referral_amount ? (
                  <DI label={t('orders.field.referral')}>
                    <MoneyText amount={order.referral_amount} currency={order.currency} />
                  </DI>
                ) : null}
              </DL>
            </CardBody>
          </Card>

          {stripeRefs && (
            <Card>
              <CardHeader title={t('orders.section.stripe')} />
              <CardBody>
                <dl className="space-y-3">
                  <StripeRef label={t('orders.field.checkoutSession')} value={order.stripe_checkout_session} />
                  <StripeRef label={t('orders.field.paymentIntent')} value={order.stripe_payment_intent} />
                  <StripeRef label={t('orders.field.invoice')} value={order.stripe_invoice_id} />
                  <StripeRef label={t('orders.field.stripeCustomer')} value={order.stripe_customer_id} />
                </dl>
              </CardBody>
            </Card>
          )}

          {hasCustomData && (
            <Card>
              <details>
                <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-4 py-3 text-sm font-semibold marker:hidden">
                  {t('orders.section.customData')}
                  <Badge>{typeof order.custom_data === 'object' ? 'JSON' : String(order.custom_data).length}</Badge>
                </summary>
                <div className="border-t border-line p-3">
                  <JsonViewer value={order.custom_data} maxHeight={280} />
                </div>
              </details>
            </Card>
          )}
        </div>
      </div>

      {dialog === 'refund' && <RefundDialog order={order} onClose={() => setDialog(null)} onDone={() => void mutate()} />}
      {dialog === 'resend' && <ResendReceiptDialog order={order} onClose={() => setDialog(null)} onDone={() => void mutate()} />}
    </>
  );
}
