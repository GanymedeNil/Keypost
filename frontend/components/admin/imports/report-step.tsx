'use client';

import { Download } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useT } from '@/lib/i18n/provider';
import { ButtonAnchor } from '@/components/kit';
import { StepCard } from './step-card';

export function ReportStep() {
  const t = useT();
  return (
    <StepCard step={4} title={t('imports.step4.title')} description={t('imports.step4.desc')}>
      <ButtonAnchor href={adminApi.url('/imports/mapping-report.csv')} download variant="secondary" icon={<Download className="size-4" />}>
        {t('imports.step4.download')}
      </ButtonAnchor>
    </StepCard>
  );
}
