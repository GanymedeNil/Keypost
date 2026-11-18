'use client';

import Link from 'next/link';
import { useI18n } from '@/lib/i18n/provider';
import type { AuditLog, EmailLog } from '@/lib/types';
import { Callout, DateText, DI, DL, Drawer, JsonViewer, Mono, StatusBadge } from '@/components/kit';

export function resourceLabel(log: AuditLog): string {
  if (!log.resource_type) return '—';
  return log.resource_id === null || log.resource_id === '' ? log.resource_type : `${log.resource_type} #${log.resource_id}`;
}

function hasDetails(details: unknown): boolean {
  if (details === null || details === undefined || details === '') return false;
  if (typeof details === 'object') return Object.keys(details as object).length > 0;
  return true;
}

export function AuditDrawer({ log, onClose }: { log: AuditLog | null; onClose: () => void }) {
  const { t } = useI18n();
  return (
    <Drawer open={log !== null} onOpenChange={(o) => !o && onClose()} title={log ? t('team.audit.drawerTitle', { id: log.id }) : ''} description={log?.action}>
      {log && (
        <div className="space-y-5">
          <DL cols={2}>
            <DI label={t('common.date')}>
              <DateText value={log.created_at} />
            </DI>
            <DI label={t('team.audit.col.user')}>{log.user_email ?? t('team.audit.system')}</DI>
            <DI label={t('team.audit.col.action')}>
              <Mono>{log.action}</Mono>
            </DI>
            <DI label={t('team.audit.col.resource')}>{resourceLabel(log) === '—' ? null : resourceLabel(log)}</DI>
            <DI label={t('team.audit.col.ip')}>{log.ip}</DI>
          </DL>
          <div className="space-y-1.5">
            <p className="text-xs font-medium">{t('team.audit.col.details')}</p>
            {hasDetails(log.details) ? <JsonViewer value={log.details} /> : <p className="text-sm text-muted-foreground">{t('team.audit.noDetails')}</p>}
          </div>
        </div>
      )}
    </Drawer>
  );
}

export function EmailDrawer({ log, onClose }: { log: EmailLog | null; onClose: () => void }) {
  const { t } = useI18n();
  return (
    <Drawer open={log !== null} onOpenChange={(o) => !o && onClose()} title={log ? t('team.email.drawerTitle', { id: log.id }) : ''} description={log?.subject}>
      {log && (
        <div className="space-y-5">
          <DL cols={2}>
            <DI label={t('common.date')}>
              <DateText value={log.created_at} />
            </DI>
            <DI label={t('common.status')}>
              <StatusBadge status={log.status} />
            </DI>
            <DI label={t('team.email.col.to')}>{log.to}</DI>
            <DI label={t('team.email.col.template')}>{log.template ? <Mono>{log.template}</Mono> : null}</DI>
            <DI label={t('team.email.col.subject')} className="sm:col-span-2">
              {log.subject}
            </DI>
            <DI label={t('team.email.col.order')}>
              {log.order_id ? (
                <Link href={`/admin/orders/${log.order_id}`} className="text-primary hover:underline">
                  {t('team.email.openOrder')} #{log.order_id}
                </Link>
              ) : null}
            </DI>
          </DL>
          <div className="space-y-1.5">
            <p className="text-xs font-medium">{t('team.email.col.error')}</p>
            {log.error ? <Callout tone="danger"><span className="break-words font-mono text-xs">{log.error}</span></Callout> : <p className="text-sm text-muted-foreground">{t('team.email.noError')}</p>}
          </div>
        </div>
      )}
    </Drawer>
  );
}
