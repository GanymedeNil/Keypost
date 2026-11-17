'use client';

import type { ReactNode } from 'react';
import { formatBytes, formatNumber } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import { isRecord } from '@/lib/utils';
import type { ImportJob } from '@/lib/types';
import { Badge, Card, CardBody, CardHeader, DateText, DI, DL } from '@/components/ui';
import { formatDuration, jobDuration, useImportLabels } from './utils';

const SECRET_KEYS = new Set(['api_key', 'apikey', 'token', 'secret']);

function OptionValue({ name, value }: { name: string; value: unknown }): ReactNode {
  const { t, te, locale } = useI18n();
  const labels = useImportLabels();
  if (SECRET_KEYS.has(name.toLowerCase())) return <span className="text-subtle">{t('imports.options.masked')}</span>;
  if (typeof value === 'boolean') return t(value ? 'common.yesNo.true' : 'common.yesNo.false');
  if (typeof value === 'number') return name === 'size' ? formatBytes(value, locale) : formatNumber(value, locale);
  if (typeof value === 'string') return name === 'kind' ? te(`imports.mapKind.${value}`, value) : value;
  if (Array.isArray(value)) {
    if (value.length === 0) return t('imports.options.all');
    return (
      <span className="flex flex-wrap gap-1">
        {value.map((v) => (
          <Badge key={String(v)}>{typeof v === 'string' ? labels.stage(v) : String(v)}</Badge>
        ))}
      </span>
    );
  }
  if (value === null || value === undefined) return null;
  return <code className="text-xs">{JSON.stringify(value)}</code>;
}

export function JobDetails({ job }: { job: ImportJob }) {
  const { t } = useI18n();
  const labels = useImportLabels();
  const ms = jobDuration(job);
  const options = isRecord(job.options) ? Object.entries(job.options) : [];
  return (
    <Card>
      <CardHeader title={t('imports.options.title')} />
      <CardBody className="space-y-5">
        <DL>
          <DI label={t('imports.options.kind')}>{labels.kind(job.kind)}</DI>
          <DI label={t('imports.options.createdBy')}>{job.created_by}</DI>
          <DI label={t('imports.options.created')}>
            <DateText value={job.created_at} />
          </DI>
          <DI label={t('imports.options.started')}>{job.started_at && <DateText value={job.started_at} />}</DI>
          <DI label={t('imports.options.finished')}>{job.finished_at && <DateText value={job.finished_at} />}</DI>
          <DI label={t('imports.options.duration')}>{ms !== null ? formatDuration(ms) : null}</DI>
        </DL>
        {options.length > 0 && (
          <DL className="border-t border-line pt-4">
            {options.map(([k, v]) => (
              <DI key={k} label={labels.option(k)}>
                <OptionValue name={k} value={v} />
              </DI>
            ))}
          </DL>
        )}
      </CardBody>
    </Card>
  );
}
