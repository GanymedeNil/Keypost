'use client';

import { useT } from '@/lib/i18n/provider';
import { useCan } from '@/components/admin/admin-context';
import { AutoMatchStep } from '@/components/admin/imports/auto-match-step';
import { JobsTable } from '@/components/admin/imports/jobs-table';
import { LemonSqueezyStep } from '@/components/admin/imports/lemonsqueezy-step';
import { MappingStep } from '@/components/admin/imports/mapping-step';
import { ReportStep } from '@/components/admin/imports/report-step';
import { Callout, PageHeader } from '@/components/kit';

export default function ImportsPage() {
  const t = useT();
  const can = useCan();
  const readOnly = !can.write;
  return (
    <>
      <PageHeader title={t('imports.title')} description={t('imports.subtitle')} />
      <div className="space-y-4">
        {readOnly && <Callout tone="info">{t('imports.readOnlyNotice')}</Callout>}
        <LemonSqueezyStep disabled={readOnly} />
        <MappingStep disabled={readOnly} />
        <AutoMatchStep disabled={readOnly} />
        <ReportStep />
        <JobsTable />
      </div>
    </>
  );
}
