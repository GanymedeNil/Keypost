'use client';

import { Button } from '@/components/ui';
import { useT } from '@/lib/i18n/provider';

/** Sticky "unsaved changes" bar. Must be rendered inside the `<form>` it submits. */
export function SaveBar({ dirty, pending, onReset, disabled }: { dirty: boolean; pending: boolean; onReset: () => void; disabled?: boolean }) {
  const t = useT();
  if (!dirty) return null;
  return (
    <div
      role="region"
      aria-label={t('settings.unsaved')}
      className="sticky bottom-4 z-20 flex animate-toast-in flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-surface px-4 py-3 shadow-[var(--shadow-pop)]"
    >
      <p className="text-sm text-muted">{t('settings.unsaved')}</p>
      <div className="flex items-center gap-2">
        <Button type="button" onClick={onReset} disabled={pending}>
          {t('common.reset')}
        </Button>
        <Button type="submit" variant="primary" loading={pending} disabled={disabled}>
          {pending ? t('common.saving') : t('common.saveChanges')}
        </Button>
      </div>
    </div>
  );
}
