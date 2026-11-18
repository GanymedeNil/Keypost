'use client';

import { useId, useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, Field, Input, Modal } from '@/components/kit';
import { useAdmin } from '@/components/admin/admin-context';

/** Sends a test message through the configured SMTP server. Mount only while open. */
export function TestEmailDialog({ onClose, onSent }: { onClose: () => void; onSent: () => void }) {
  const { t } = useI18n();
  const { user } = useAdmin();
  const formId = useId();
  const [to, setTo] = useState(user.email);
  const { run, pending, fieldErrors } = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const email = to.trim();
    const res = await run(() => adminApi.post('/email-logs/test', { to: email }), { success: t('team.email.testSent', { email }) });
    if (res.ok) onSent();
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && !pending && onClose()}
      title={t('team.email.sendTest')}
      description={t('team.email.testDesc')}
      size="sm"
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending} disabled={!to.trim()}>
            {t('common.send')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate>
        <Field label={t('team.email.testTo')} error={fieldErrors.to} required>
          <Input type="email" value={to} onChange={(e) => setTo(e.target.value)} autoFocus />
        </Field>
      </form>
    </Modal>
  );
}
