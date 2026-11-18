'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import { useAction } from '@/lib/use-action';
import type { OrderDetail } from '@/lib/types';
import { Button, Field, Input, Modal } from '@/components/kit';

/** Mount only while open (fresh state on every open). */
export function ResendReceiptDialog({ order, onClose, onDone }: { order: OrderDetail; onClose: () => void; onDone: () => void }) {
  const { t } = useI18n();
  const { run, pending, fieldErrors } = useAction();
  const [email, setEmail] = useState('');

  async function submit(e: FormEvent) {
    e.preventDefault();
    const body = email.trim() ? { email: email.trim() } : {};
    const res = await run(() => adminApi.post(`/orders/${order.id}/resend-receipt`, body), { success: t('orders.resend.success') });
    if (res.ok) {
      await invalidate('/orders');
      onDone();
      onClose();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('orders.resend.title')}
      description={t('orders.resend.description', { number: order.order_number })}
      size="sm"
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="resend-form" variant="primary" loading={pending}>
            {t('orders.resend.submit')}
          </Button>
        </>
      }
    >
      <form id="resend-form" onSubmit={submit} noValidate>
        <Field label={t('orders.resend.email')} hint={t('orders.resend.hint', { email: order.user_email })} error={fieldErrors.email} optional>
          <Input type="email" autoFocus autoComplete="off" placeholder={order.user_email} value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
      </form>
    </Modal>
  );
}
