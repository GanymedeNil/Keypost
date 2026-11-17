'use client';

import { useI18n } from '@/lib/i18n/provider';
import type { Delivery } from '@/lib/types';
import { Button, ButtonLink, Callout, DI, DL, Modal, StatusBadge } from '@/components/ui';
import { CodeBlock } from './code-block';

/** Result of "Send test": the synchronous Delivery returned by `POST /webhooks/{id}/test`. */
export function TestResultModal({ delivery, onClose }: { delivery: Delivery; onClose: () => void }) {
  const { t } = useI18n();
  const body = delivery.last_response?.trim();
  const tone = delivery.status === 'success' ? 'success' : delivery.status === 'failed' ? 'danger' : 'info';
  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('webhooks.test.title')}
      description={t('webhooks.test.description', { url: delivery.webhook_url })}
      footer={
        <>
          <ButtonLink href={`/admin/webhooks/deliveries?webhook_id=${delivery.webhook_id}&delivery=${delivery.id}`} onClick={onClose}>
            {t('webhooks.viewDeliveries')}
          </ButtonLink>
          <Button variant="primary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Callout tone={tone}>
          {delivery.status === 'success' ? t('webhooks.test.success') : delivery.status === 'failed' ? t('webhooks.test.failed') : t('webhooks.test.pending')}
        </Callout>
        <DL cols={2}>
          <DI label={t('common.status')}>
            <StatusBadge status={delivery.status} />
          </DI>
          <DI label={t('webhooks.test.statusCode')}>{delivery.last_status_code ?? '—'}</DI>
          <DI label={t('webhooks.test.duration')}>{delivery.duration_ms === null ? '—' : t('webhooks.durationMs', { ms: delivery.duration_ms })}</DI>
          <DI label={t('webhooks.test.attempts')}>{delivery.attempts}</DI>
        </DL>
        {delivery.last_error && (
          <div>
            <p className="mb-1 text-xs font-medium text-fg">{t('webhooks.drawer.error')}</p>
            <CodeBlock value={delivery.last_error} maxHeight={140} />
          </div>
        )}
        <div>
          <p className="mb-1 text-xs font-medium text-fg">{t('common.response')}</p>
          {body ? <CodeBlock value={body} /> : <p className="text-xs text-subtle">{t('webhooks.test.noResponse')}</p>}
        </div>
      </div>
    </Modal>
  );
}
