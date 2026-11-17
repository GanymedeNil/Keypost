'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import type { VariantFile } from '@/lib/types';
import { Button, Field, Input, Modal, Select } from '@/components/ui';

interface Props {
  file: VariantFile | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void | Promise<unknown>;
}

function Inner({ file, onOpenChange, onSaved }: { file: VariantFile; onOpenChange: (open: boolean) => void; onSaved: Props['onSaved'] }) {
  const { t, te } = useI18n();
  const [name, setName] = useState(file.name);
  const [version, setVersion] = useState(file.version ?? '');
  const [sort, setSort] = useState(String(file.sort));
  const [status, setStatus] = useState<string>(file.status);
  const { run, pending, fieldErrors } = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const n = Number(sort);
    const res = await run(
      () =>
        adminApi.patch<VariantFile>(`/files/${file.id}`, {
          name: name.trim(),
          version: version.trim(),
          sort: Number.isFinite(n) ? n : file.sort,
          status,
        }),
      { success: t('toast.saved') },
    );
    if (res.ok) {
      await onSaved();
      onOpenChange(false);
    }
  }

  return (
    <Modal
      open
      onOpenChange={onOpenChange}
      dismissible={!pending}
      title={t('products.files.editTitle')}
      description={`${file.name}.${file.extension}`}
      footer={
        <>
          <Button onClick={() => onOpenChange(false)} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="file-edit-form" variant="primary" loading={pending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="file-edit-form" onSubmit={submit} noValidate className="space-y-4">
        <Field label={t('common.name')} required error={fieldErrors.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('common.version')} error={fieldErrors.version}>
            <Input value={version} onChange={(e) => setVersion(e.target.value)} placeholder="1.0.0" />
          </Field>
          <Field label={t('common.sort')} error={fieldErrors.sort}>
            <Input type="number" step={1} value={sort} onChange={(e) => setSort(e.target.value)} />
          </Field>
        </div>
        <Field label={t('common.status')} error={fieldErrors.status}>
          <Select value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="published">{te('status.published', 'Published')}</option>
            <option value="draft">{te('status.draft', 'Draft')}</option>
          </Select>
        </Field>
      </form>
    </Modal>
  );
}

export function FileEditDialog(props: Props) {
  return props.file ? <Inner key={props.file.id} file={props.file} onOpenChange={props.onOpenChange} onSaved={props.onSaved} /> : null;
}
