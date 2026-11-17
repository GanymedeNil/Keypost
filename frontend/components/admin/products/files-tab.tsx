'use client';

import { useState } from 'react';
import { Download, Pencil, Trash2 } from 'lucide-react';
import { adminApi, type ApiError } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import type { VariantFile } from '@/lib/types';
import { Badge, Button, CopyButton, DataTable, Mono, StatusBadge, useConfirmDelete, type Column } from '@/components/ui';
import { FileEditDialog } from './file-edit-dialog';
import { FileUploader } from './file-uploader';
import type { UploadItem, UploadQueue } from './upload-queue';

export function FilesTab({
  files,
  loading,
  error,
  onRetry,
  queue,
  items,
  onChanged,
}: {
  files: VariantFile[] | undefined;
  loading: boolean;
  error?: ApiError;
  onRetry: () => void;
  queue: UploadQueue;
  items: UploadItem[];
  onChanged: () => Promise<unknown>;
}) {
  const { t } = useI18n();
  const can = useCan();
  const confirmDelete = useConfirmDelete();
  const { run } = useAction();
  const [editing, setEditing] = useState<VariantFile | null>(null);

  async function toggleStatus(file: VariantFile) {
    const next = file.status === 'published' ? 'draft' : 'published';
    const res = await run(() => adminApi.patch(`/files/${file.id}`, { status: next }), { success: t('toast.updated') });
    if (res.ok) await onChanged();
  }

  async function remove(file: VariantFile) {
    if (!(await confirmDelete(`${file.name}.${file.extension}`, t('products.files.deleteBody')))) return;
    const res = await run(() => adminApi.delete(`/files/${file.id}`), { success: t('toast.deleted') });
    if (res.ok) await onChanged();
  }

  const statusCell = (f: VariantFile) =>
    can.write ? (
      <button
        type="button"
        onClick={() => toggleStatus(f)}
        title={t('products.files.toggleStatus')}
        className="rounded-full focus-visible:outline-2 focus-visible:outline-accent"
      >
        <StatusBadge status={f.status} />
      </button>
    ) : (
      <StatusBadge status={f.status} />
    );

  const columns: Column<VariantFile>[] = [
    {
      key: 'name',
      header: t('common.name'),
      cell: (f) => (
        <div className="min-w-0">
          <p className="truncate font-medium">
            {f.name}
            {f.extension && <span className="font-normal text-subtle">.{f.extension}</span>}
          </p>
          <p className="text-xs text-subtle">{f.size_formatted}</p>
          <div className="mt-1 sm:hidden">{statusCell(f)}</div>
        </div>
      ),
    },
    { key: 'version', header: t('common.version'), hideBelow: 'sm', cell: (f) => f.version || <span className="text-subtle">—</span> },
    { key: 'status', header: t('common.status'), hideBelow: 'sm', cell: statusCell },
    { key: 'downloads', header: t('products.files.downloads'), align: 'right', hideBelow: 'md', cell: (f) => <span className="tabular-nums">{f.downloads_count}</span> },
    {
      key: 'sha',
      header: 'SHA-256',
      hideBelow: 'lg',
      cell: (f) =>
        f.sha256 ? (
          <span className="inline-flex items-center gap-1">
            <span title={f.sha256}>
              <Mono>{f.sha256.slice(0, 10)}…</Mono>
            </span>
            <CopyButton value={f.sha256} label="SHA-256" />
          </span>
        ) : (
          <span className="text-subtle">—</span>
        ),
    },
    { key: 'sort', header: t('common.sort'), align: 'right', hideBelow: 'lg', cell: (f) => <span className="tabular-nums">{f.sort}</span> },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      align: 'right',
      cell: (f) => (
        <div className="flex items-center justify-end gap-0.5">
          <a
            href={adminApi.url(`/files/${f.id}/download`)}
            aria-label={t('common.download')}
            title={t('common.download')}
            className="inline-flex size-7 items-center justify-center rounded-md text-muted hover:bg-surface-2 hover:text-fg"
          >
            <Download className="size-3.5" />
          </a>
          {can.write && (
            <>
              <Button size="icon-sm" variant="ghost" aria-label={t('common.edit')} onClick={() => setEditing(f)}>
                <Pencil className="size-3.5" />
              </Button>
              <Button size="icon-sm" variant="ghost" aria-label={t('common.delete')} className="hover:text-danger-text" onClick={() => remove(f)}>
                <Trash2 className="size-3.5" />
              </Button>
            </>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      {can.write && <FileUploader queue={queue} items={items} onUploaded={() => void onChanged()} />}
      <div className="overflow-hidden rounded-lg border border-line">
        <div className="flex items-center justify-between border-b border-line bg-surface-2/60 px-3 py-2">
          <h3 className="text-xs font-medium text-muted">{t('products.files.title')}</h3>
          {files && <Badge>{files.length}</Badge>}
        </div>
        <DataTable
          columns={columns}
          rows={files}
          rowKey={(f) => f.id}
          loading={loading}
          error={error}
          onRetry={onRetry}
          skeletonRows={3}
          empty={{ title: t('products.files.empty'), description: t('products.files.emptyHint') }}
        />
      </div>
      <FileEditDialog file={editing} onOpenChange={(o) => !o && setEditing(null)} onSaved={onChanged} />
    </div>
  );
}
