'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { formatMoney, majorToMinor, minorToMajor } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import { useAction } from '@/lib/use-action';
import type { OrderDetail } from '@/lib/types';
import { Button, Callout, Field, InputGroup, Modal, Select } from '@/components/kit';

const REASONS = ['requested_by_customer', 'duplicate', 'fraudulent'] as const;

interface Props {
  order: OrderDetail;
  onClose: () => void;
  onDone: () => void;
}

/** Mount only while open: its state is initialised fresh every time the dialog opens. */
export function RefundDialog({ order, onClose, onDone }: Props) {
  const { t, te, locale } = useI18n();
  const { run, pending, fieldErrors } = useAction();
  const remaining = Math.max(0, order.total - order.refunded_amount);
  const money = (cents: number) => formatMoney(cents, order.currency, locale);
  const step = minorToMajor(1, order.currency, locale);

  const [mode, setMode] = useState<'full' | 'partial'>('full');
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [amountError, setAmountError] = useState<string | null>(null);

  const nonStripe = order.source !== 'stripe' || !order.stripe_payment_intent;
  const parsed = amount === '' ? NaN : Number(amount);
  const partialMinor = Number.isFinite(parsed) ? majorToMinor(parsed, order.currency, locale) : 0;
  const submitCents = mode === 'full' ? remaining : partialMinor;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (mode === 'partial' && (partialMinor < 1 || partialMinor > remaining)) {
      setAmountError(t('orders.refund.invalidAmount', { min: money(1), max: money(remaining) }));
      return;
    }
    setAmountError(null);
    const body: { amount?: number; reason?: string } = {};
    if (mode === 'partial') body.amount = partialMinor;
    if (reason) body.reason = reason;
    const res = await run(() => adminApi.post<OrderDetail>(`/orders/${order.id}/refund`, body), { success: t('orders.refund.success') });
    if (res.ok) {
      await invalidate('/orders', '/dashboard', '/customers', '/license-keys');
      onDone();
      onClose();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('orders.refund.title', { number: order.order_number })}
      description={t('orders.refund.description')}
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="refund-form" variant="danger" loading={pending} disabled={submitCents < 1}>
            {t('orders.refund.submit', { amount: money(submitCents) })}
          </Button>
        </>
      }
    >
      <form id="refund-form" onSubmit={submit} className="space-y-4" noValidate>
        {nonStripe && <Callout tone="warning">{t('orders.refund.nonStripe', { source: te(`source.${order.source}`, order.source) })}</Callout>}

        <fieldset className="space-y-2">
          <legend className="sr-only">{t('orders.refund.title', { number: order.order_number })}</legend>
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <input type="radio" name="refund-mode" className="size-4 accent-primary" checked={mode === 'full'} onChange={() => setMode('full')} />
            {t('orders.refund.full', { amount: money(remaining) })}
          </label>
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <input type="radio" name="refund-mode" className="size-4 accent-primary" checked={mode === 'partial'} onChange={() => setMode('partial')} />
            {t('orders.refund.partial')}
          </label>
        </fieldset>

        {mode === 'partial' && (
          <Field
            label={t('orders.refund.amount')}
            hint={
              order.refunded_amount > 0
                ? `${t('orders.refund.alreadyRefunded', { amount: money(order.refunded_amount) })} ${t('orders.refund.maxHint', { amount: money(remaining) })}`
                : t('orders.refund.maxHint', { amount: money(remaining) })
            }
            error={amountError ?? fieldErrors.amount}
            required
          >
            <InputGroup
              type="number"
              inputMode="decimal"
              min={step}
              max={minorToMajor(remaining, order.currency, locale)}
              step={step}
              prefix={order.currency.toUpperCase()}
              value={amount}
              autoFocus
              onChange={(e) => setAmount(e.target.value)}
            />
          </Field>
        )}

        <Field label={t('orders.refund.reason')} optional>
          <Select value={reason} onChange={(e) => setReason(e.target.value)}>
            <option value="">{t('orders.refund.reasonNone')}</option>
            {REASONS.map((r) => (
              <option key={r} value={r}>
                {te(`orders.refund.reason.${r}`, r)}
              </option>
            ))}
          </Select>
        </Field>
      </form>
    </Modal>
  );
}
