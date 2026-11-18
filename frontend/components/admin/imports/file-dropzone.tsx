'use client';

import { useRef, useState } from 'react';
import { FileText, UploadCloud, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { formatBytes } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import { Button } from '@/components/kit';

/** Single-file drop target with click-to-browse. `extensions` are lower-case, with the dot (e.g. `.zip`). */
export function FileDropzone({
  file,
  onFile,
  extensions,
  prompt,
  error,
  disabled,
}: {
  file: File | null;
  onFile: (file: File | null) => void;
  extensions: string[];
  prompt: string;
  error?: string | null;
  disabled?: boolean;
}) {
  const { t, locale } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [badType, setBadType] = useState(false);

  function accept(f: File | undefined) {
    if (!f) return;
    const ok = extensions.some((ext) => f.name.toLowerCase().endsWith(ext));
    setBadType(!ok);
    if (ok) onFile(f);
  }

  const message = badType ? t('imports.dropzone.badType', { types: extensions.join(', ') }) : error;

  return (
    <div>
      <input
        ref={inputRef}
        type="file"
        hidden
        accept={extensions.join(',')}
        onChange={(e) => {
          accept(e.target.files?.[0]);
          e.target.value = '';
        }}
      />
      {file ? (
        <div className="flex items-center gap-3 rounded-lg border border-input bg-muted px-3 py-2.5">
          <FileText aria-hidden className="size-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-foreground">{file.name}</p>
            <p className="text-xs text-muted-foreground">{formatBytes(file.size, locale)}</p>
          </div>
          <Button size="sm" variant="ghost" onClick={() => inputRef.current?.click()} disabled={disabled}>
            {t('imports.dropzone.replace')}
          </Button>
          <Button size="icon-sm" variant="ghost" aria-label={t('imports.dropzone.remove')} onClick={() => onFile(null)} disabled={disabled}>
            <X className="size-4" />
          </Button>
        </div>
      ) : (
        <button
          type="button"
          disabled={disabled}
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(true);
          }}
          onDragLeave={() => setOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setOver(false);
            accept(e.dataTransfer.files[0]);
          }}
          className={cn(
            'flex w-full flex-col items-center gap-1.5 rounded-lg border border-dashed px-4 py-7 text-center transition-colors',
            over ? 'border-primary bg-accent' : 'border-input bg-muted/50 hover:border-muted-foreground hover:bg-muted',
            message && 'border-danger',
          )}
        >
          <UploadCloud aria-hidden className="size-6 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">{prompt}</span>
        </button>
      )}
      {message && (
        <p role="alert" className="mt-1.5 text-xs text-danger-text">
          {message}
        </p>
      )}
    </div>
  );
}
