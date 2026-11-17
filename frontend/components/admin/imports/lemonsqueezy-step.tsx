'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import type { ImportJob } from '@/lib/types';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useAdmin } from '@/components/admin/admin-context';
import { Button, Callout, Checkbox, Field, Input, Progress, SwitchField } from '@/components/ui';
import { FileDropzone } from './file-dropzone';
import { Segmented } from './segmented';
import { StepCard } from './step-card';
import { LS_STAGES, RESOURCE_DEPS, useImportLabels, useOpenJob, type LsResource } from './utils';

type Source = 'api' | 'archive';

function parseStoreIds(raw: string): number[] | null {
  const parts = raw.split(/[\s,]+/).filter(Boolean);
  const ids = parts.map((p) => (/^\d+$/.test(p) ? Number(p) : NaN));
  return ids.some((n) => !Number.isSafeInteger(n) || n <= 0) ? null : ids;
}

function ApiForm({ disabled }: { disabled: boolean }) {
  const t = useT();
  const labels = useImportLabels();
  const { testMode } = useAdmin();
  const openJob = useOpenJob();
  const { run, pending, fieldErrors } = useAction();

  const [apiKey, setApiKey] = useState('');
  const [lsTestMode, setLsTestMode] = useState(testMode);
  const [storeIds, setStoreIds] = useState('');
  const [resources, setResources] = useState<LsResource[]>([...LS_STAGES]);
  const [downloadFiles, setDownloadFiles] = useState(true);
  const [dryRun, setDryRun] = useState(false);
  const [errors, setErrors] = useState<{ apiKey?: string; storeIds?: string; resources?: string }>({});

  const toggle = (r: LsResource, on: boolean) => setResources((cur) => (on ? LS_STAGES.filter((x) => x === r || cur.includes(x)) : cur.filter((x) => x !== r)));

  const depWarnings = resources.flatMap((r) => {
    const missing = RESOURCE_DEPS[r].filter((d) => !resources.includes(d));
    return missing.length ? [{ resource: r, missing }] : [];
  });

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: typeof errors = {};
    if (!apiKey.trim()) next.apiKey = t('imports.err.apiKey');
    const ids = storeIds.trim() ? parseStoreIds(storeIds) : [];
    if (ids === null) next.storeIds = t('imports.err.storeIds');
    if (resources.length === 0) next.resources = t('imports.err.resources');
    setErrors(next);
    if (Object.keys(next).length > 0) return;

    const body = {
      api_key: apiKey.trim(),
      test_mode: lsTestMode,
      ...(ids && ids.length > 0 ? { store_ids: ids } : {}),
      ...(resources.length < LS_STAGES.length ? { resources } : {}),
      download_files: downloadFiles,
      dry_run: dryRun,
    };
    const res = await run(() => adminApi.post<ImportJob>('/imports/lemonsqueezy', body), { success: t('imports.toast.started') });
    if (res.ok) {
      setApiKey('');
      openJob(res.data);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-5" noValidate>
      <Field label={t('imports.api.key')} hint={t('imports.api.keyHint')} error={errors.apiKey ?? fieldErrors.api_key} required>
        <Input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
          className="font-mono"
        />
      </Field>

      <div className="grid gap-5 sm:grid-cols-2">
        <Field label={t('imports.api.storeIds')} hint={t('imports.api.storeIdsHint')} error={errors.storeIds ?? fieldErrors.store_ids} optional>
          <Input
            value={storeIds}
            onChange={(e) => setStoreIds(e.target.value)}
            placeholder={t('imports.api.storeIdsPlaceholder')}
            inputMode="numeric"
            autoComplete="off"
          />
        </Field>
        <SwitchField
          label={t('imports.api.testMode')}
          description={t('imports.api.testModeDesc')}
          checked={lsTestMode}
          onCheckedChange={setLsTestMode}
        />
      </div>

      <fieldset className="min-w-0 border-0 p-0">
        <legend className="mb-2 flex w-full flex-wrap items-center justify-between gap-2 p-0 text-xs font-medium text-fg">
          {t('imports.api.resources')}
          <span className="flex gap-1">
            <Button size="sm" variant="ghost" onClick={() => setResources([...LS_STAGES])}>
              {t('imports.api.selectAll')}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setResources([])}>
              {t('imports.api.selectNone')}
            </Button>
          </span>
        </legend>
        <div className="grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3">
          {LS_STAGES.map((r) => (
            <Checkbox key={r} label={labels.stage(r)} checked={resources.includes(r)} onCheckedChange={(on) => toggle(r, on)} />
          ))}
        </div>
        {errors.resources && (
          <p role="alert" className="mt-1.5 text-xs text-danger-text">
            {errors.resources}
          </p>
        )}
        {depWarnings.length > 0 && (
          <Callout tone="warning" className="mt-3 text-xs">
            <ul className="space-y-0.5">
              {depWarnings.map((w) => (
                <li key={w.resource}>
                  {t('imports.api.depsWarning', { resource: labels.stage(w.resource), needs: w.missing.map(labels.stage).join(', ') })}
                </li>
              ))}
            </ul>
          </Callout>
        )}
      </fieldset>

      <div className="grid gap-5 sm:grid-cols-2">
        <SwitchField
          label={t('imports.downloadFiles')}
          description={t('imports.downloadFilesDesc')}
          checked={downloadFiles}
          onCheckedChange={setDownloadFiles}
        />
        <SwitchField label={t('imports.dryRun')} description={t('imports.dryRunDesc')} checked={dryRun} onCheckedChange={setDryRun} />
      </div>

      <Button type="submit" variant="primary" loading={pending} disabled={disabled}>
        {dryRun ? t('imports.api.startDry') : t('imports.api.start')}
      </Button>
    </form>
  );
}

function ArchiveForm({ disabled }: { disabled: boolean }) {
  const t = useT();
  const openJob = useOpenJob();
  const { run, pending, fieldErrors } = useAction();
  const [file, setFile] = useState<File | null>(null);
  const [downloadFiles, setDownloadFiles] = useState(true);
  const [percent, setPercent] = useState<number | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!file) return setFileError(t('imports.err.file'));
    setFileError(null);
    const form = new FormData();
    form.append('file', file);
    form.append('download_files', String(downloadFiles));
    setPercent(0);
    const res = await run(
      () => adminApi.upload<ImportJob>('/imports/lemonsqueezy/archive', form, { onProgress: (loaded, total) => setPercent(Math.round((loaded / total) * 100)) }),
      { success: t('imports.toast.started') },
    );
    setPercent(null);
    if (res.ok) openJob(res.data);
  }

  return (
    <form onSubmit={submit} className="space-y-5" noValidate>
      <Field label={t('imports.archive.file')} error={fileError ?? fieldErrors.file} required>
        <FileDropzone
          file={file}
          onFile={(f) => {
            setFile(f);
            setFileError(null);
          }}
          extensions={['.zip']}
          prompt={t('imports.dropzone.zip')}
          disabled={disabled || pending}
        />
      </Field>
      <SwitchField
        label={t('imports.downloadFiles')}
        description={t('imports.downloadFilesDesc')}
        checked={downloadFiles}
        onCheckedChange={setDownloadFiles}
      />
      {percent !== null && (
        <div className="space-y-1.5" role="status">
          <Progress value={percent} label={t('imports.uploading', { percent })} />
          <p className="text-xs text-muted">{percent >= 100 ? t('imports.processing') : t('imports.uploading', { percent })}</p>
        </div>
      )}
      <Button type="submit" variant="primary" loading={pending} disabled={disabled}>
        {t('imports.archive.start')}
      </Button>
    </form>
  );
}

export function LemonSqueezyStep({ disabled }: { disabled: boolean }) {
  const t = useT();
  const [source, setSource] = useState<Source>('api');
  return (
    <StepCard step={1} title={t('imports.step1.title')} description={t('imports.step1.desc')} disabled={disabled}>
      <div className="mb-5 space-y-3">
        <Segmented
          ariaLabel={t('imports.source.label')}
          value={source}
          onChange={setSource}
          options={[
            { value: 'api', label: t('imports.source.api') },
            { value: 'archive', label: t('imports.source.archive') },
          ]}
        />
        {source === 'archive' && <p className="max-w-3xl text-sm text-muted">{t('imports.archive.desc')}</p>}
      </div>
      {source === 'api' ? <ApiForm disabled={disabled} /> : <ArchiveForm disabled={disabled} />}
    </StepCard>
  );
}
