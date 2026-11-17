'use client';

import { useRef, useState, type DragEvent } from 'react';
import { CheckCircle2, RotateCcw, UploadCloud, X, XCircle } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { errorText } from '@/lib/error-text';
import { formatBytes } from '@/lib/format';
import { cn } from '@/lib/utils';
import { Button, Field, Input, Progress, Select } from '@/components/ui';
import type { UploadOptions, UploadQueue, UploadItem } from './upload-queue';

function UploadRow({ item, queue }: { item: UploadItem; queue: UploadQueue }) {
  const { t, locale } = useI18n();
  const finished = item.state === 'done' || item.state === 'error' || item.state === 'cancelled';
  return (
    <li className="space-y-1.5 px-3 py-2.5">
      <div className="flex items-center gap-2">
        {item.state === 'done' ? (
          <CheckCircle2 className="size-4 shrink-0 text-success" />
        ) : item.state === 'error' || item.state === 'cancelled' ? (
          <XCircle className="size-4 shrink-0 text-danger" />
        ) : (
          <UploadCloud className="size-4 shrink-0 text-subtle" />
        )}
        <span className="min-w-0 flex-1 truncate text-sm" title={item.file.name}>
          {item.file.name}
        </span>
        <span className="shrink-0 text-xs tabular-nums text-subtle">{formatBytes(item.file.size, locale)}</span>
        {(item.state === 'error' || item.state === 'cancelled') && (
          <Button size="icon-sm" variant="ghost" aria-label={t('common.retry')} title={t('common.retry')} onClick={() => queue.retry(item.id)}>
            <RotateCcw className="size-3.5" />
          </Button>
        )}
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={finished ? t('common.remove') : t('common.cancel')}
          title={finished ? t('common.remove') : t('common.cancel')}
          onClick={() => (finished ? queue.remove(item.id) : queue.cancel(item.id))}
        >
          <X className="size-3.5" />
        </Button>
      </div>
      {(item.state === 'uploading' || item.state === 'queued') && (
        <div className="flex items-center gap-2">
          <Progress value={item.progress} className="flex-1" label={item.file.name} />
          <span className="w-24 shrink-0 text-right text-xs tabular-nums text-subtle">
            {item.state === 'queued' ? t('products.files.queued') : item.progress >= 100 ? t('products.files.processing') : `${item.progress}%`}
          </span>
        </div>
      )}
      {item.state === 'error' && <p className="text-xs text-danger-text">{errorText(item.error, t)}</p>}
      {item.state === 'cancelled' && <p className="text-xs text-subtle">{t('products.files.cancelled')}</p>}
    </li>
  );
}

/** Drag & drop (or click) multi-file uploader with version/status options and live progress. */
export function FileUploader({
  queue,
  items,
  onUploaded,
}: {
  queue: UploadQueue;
  items: UploadItem[];
  onUploaded: () => void;
}) {
  const { t, te } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [version, setVersion] = useState('');
  const [status, setStatus] = useState<UploadOptions['status']>('published');

  function addFiles(list: FileList | File[] | null) {
    const files = list ? Array.from(list) : [];
    if (files.length > 0) queue.add(files, { version, status }, onUploaded);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    addFiles(e.dataTransfer.files);
  }

  const hasFinished = items.some((i) => i.state === 'done' || i.state === 'error' || i.state === 'cancelled');

  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t('products.files.uploadVersion')} hint={t('products.files.uploadVersionHint')} optional>
          <Input value={version} onChange={(e) => setVersion(e.target.value)} placeholder="1.0.0" />
        </Field>
        <Field label={t('products.files.uploadStatus')} hint={t('products.files.uploadStatusHint')}>
          <Select value={status} onChange={(e) => setStatus(e.target.value as UploadOptions['status'])}>
            <option value="published">{te('status.published', 'Published')}</option>
            <option value="draft">{te('status.draft', 'Draft')}</option>
          </Select>
        </Field>
      </div>

      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'rounded-lg border-2 border-dashed transition-colors',
          dragging ? 'border-accent bg-accent-soft' : 'border-line-strong hover:border-subtle',
        )}
      >
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          className="flex w-full flex-col items-center gap-1.5 rounded-lg px-4 py-7 text-center"
        >
          <UploadCloud className={cn('size-6', dragging ? 'text-accent-text' : 'text-subtle')} />
          <span className="text-sm font-medium">{t('common.dragDrop')}</span>
          <span className="text-xs text-subtle">{t('products.files.dropHint')}</span>
        </button>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="sr-only"
          tabIndex={-1}
          aria-hidden
          onChange={(e) => {
            addFiles(e.target.files);
            e.target.value = '';
          }}
        />
      </div>

      {items.length > 0 && (
        <div className="rounded-lg border border-line">
          <div className="flex items-center justify-between border-b border-line px-3 py-2">
            <span className="text-xs font-medium text-muted">{t('products.files.uploads', { count: items.length })}</span>
            {hasFinished && (
              <Button size="sm" variant="ghost" onClick={() => queue.clearFinished()}>
                {t('products.files.clearFinished')}
              </Button>
            )}
          </div>
          <ul className="divide-y divide-line">
            {items.map((item) => (
              <UploadRow key={item.id} item={item} queue={queue} />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
