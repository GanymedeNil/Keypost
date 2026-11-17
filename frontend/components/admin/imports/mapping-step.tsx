'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import type { ImportJob } from '@/lib/types';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, Field, Progress, Select } from '@/components/ui';
import { FileDropzone } from './file-dropzone';
import { StepCard } from './step-card';
import { useOpenJob } from './utils';

const KINDS = ['customers', 'products', 'subscriptions'] as const;
type Kind = (typeof KINDS)[number];

export function MappingStep({ disabled }: { disabled: boolean }) {
  const t = useT();
  const openJob = useOpenJob();
  const { run, pending, fieldErrors } = useAction();
  const [file, setFile] = useState<File | null>(null);
  const [kind, setKind] = useState<Kind>('customers');
  const [fileError, setFileError] = useState<string | null>(null);
  const [percent, setPercent] = useState<number | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!file) return setFileError(t('imports.err.file'));
    setFileError(null);
    const form = new FormData();
    form.append('file', file);
    form.append('kind', kind);
    setPercent(0);
    const res = await run(
      () => adminApi.upload<ImportJob | undefined>('/imports/stripe-mapping', form, { onProgress: (loaded, total) => setPercent(Math.round((loaded / total) * 100)) }),
      { success: t('imports.toast.mappingUploaded') },
    );
    setPercent(null);
    if (res.ok) {
      setFile(null);
      openJob(res.data);
    }
  }

  return (
    <StepCard step={2} title={t('imports.step2.title')} description={t('imports.step2.desc')} disabled={disabled}>
      <form onSubmit={submit} className="space-y-5" noValidate>
        <Field label={t('imports.step2.kind')} hint={t('imports.step2.kindHint')} error={fieldErrors.kind} className="sm:max-w-xs">
          <Select value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {t(`imports.mapKind.${k}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('imports.step2.file')} error={fileError ?? fieldErrors.file} required>
          <FileDropzone
            file={file}
            onFile={(f) => {
              setFile(f);
              setFileError(null);
            }}
            extensions={['.csv']}
            prompt={t('imports.dropzone.csv')}
            disabled={disabled || pending}
          />
        </Field>
        {percent !== null && <Progress value={percent} label={t('imports.uploading', { percent })} />}
        <Button type="submit" variant="primary" loading={pending} disabled={disabled}>
          {t('imports.step2.start')}
        </Button>
      </form>
    </StepCard>
  );
}
