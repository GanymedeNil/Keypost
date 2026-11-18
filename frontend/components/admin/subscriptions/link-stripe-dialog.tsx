'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, Field, Input, Modal } from '@/components/kit';

/** Mount only while open so the input resets every time. */
export function LinkStripeDialog({
  subscriptionId,
  initialValue,
  onClose,
  onLinked,
}: {
  subscriptionId: number;
  initialValue: string;
  onClose: () => void;
  onLinked: () => void;
}) {
  const t = useT();
  const [value, setValue] = useState(initialValue);
  const [localError, setLocalError] = useState<string | null>(null);
  const { run, pending, fieldErrors } = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const id = value.trim();
    if (!/^sub_\w+$/.test(id)) return setLocalError(t('subscriptions.link.invalid'));
    setLocalError(null);
    const res = await run(
      () => adminApi.post(`/subscriptions/${subscriptionId}/link-stripe`, { stripe_subscription_id: id }),
      { success: t('subscriptions.toast.linked') },
    );
    if (res.ok) onLinked();
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('subscriptions.link.title')}
      description={t('subscriptions.link.description')}
      size="sm"
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="link-stripe-form" variant="primary" loading={pending}>
            {t('subscriptions.link.submit')}
          </Button>
        </>
      }
    >
      <form id="link-stripe-form" onSubmit={submit} noValidate>
        <Field label={t('subscriptions.link.label')} error={localError ?? fieldErrors.stripe_subscription_id} required>
          <Input
            autoFocus
            value={value}
            placeholder={t('subscriptions.link.placeholder')}
            onChange={(e) => setValue(e.target.value)}
            className="font-mono"
            autoComplete="off"
            spellCheck={false}
          />
        </Field>
      </form>
    </Modal>
  );
}
