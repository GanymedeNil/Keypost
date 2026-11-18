'use client';

import { FileQuestion } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { ButtonLink, EmptyState, usePageTitle } from '@/components/kit';

export default function NotFound() {
  const { t } = useI18n();
  usePageTitle(t('shell.notFound.title'));
  return (
    <div className="grid min-h-dvh place-items-center p-6">
      <EmptyState
        icon={<FileQuestion className="size-5" />}
        title={t('shell.notFound.title')}
        description={t('shell.notFound.body')}
        action={
          <div className="flex gap-2">
            <ButtonLink href="/my-orders" variant="secondary">
              {t('shell.notFound.home')}
            </ButtonLink>
            <ButtonLink href="/admin" variant="primary">
              {t('shell.notFound.admin')}
            </ButtonLink>
          </div>
        }
      />
    </div>
  );
}
