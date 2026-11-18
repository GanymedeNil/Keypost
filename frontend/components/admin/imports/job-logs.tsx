'use client';

import { useMemo, useRef, useState } from 'react';
import { Download } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useI18n } from '@/lib/i18n/provider';
import type { ImportLog } from '@/lib/types';
import { Button, Card, CardHeader, CopyButton, DateText, EmptyState, FilterSelect, Switch } from '@/components/kit';
import { useJobLogs } from './use-job-logs';
import { useScrollToEnd } from './use-scroll-to-end';

type Level = 'info' | 'warn' | 'error';
const MAX_RENDERED = 1000;

function levelOf(log: ImportLog): Level {
  if (log.level === 'error') return 'error';
  if (log.level === 'warn' || log.level === 'warning') return 'warn';
  return 'info';
}

const levelStyle: Record<Level, string> = {
  info: 'text-info-text',
  warn: 'text-warning-text',
  error: 'text-danger-text',
};

function toText(logs: ImportLog[]): string {
  return logs.map((l) => `${l.created_at} [${levelOf(l).toUpperCase()}] ${l.message}`).join('\n');
}

export function JobLogs({ jobId, live }: { jobId: number; live: boolean }) {
  const { t } = useI18n();
  const { logs, error } = useJobLogs(jobId, live);
  const [level, setLevel] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const boxRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => (level ? logs.filter((l) => levelOf(l) === level) : logs), [logs, level]);
  const rendered = filtered.length > MAX_RENDERED ? filtered.slice(-MAX_RENDERED) : filtered;
  const text = useMemo(() => toText(logs), [logs]);

  useScrollToEnd(boxRef, autoScroll, rendered.length);

  function onScroll() {
    const el = boxRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
    setAutoScroll(atBottom);
  }

  function download() {
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `import-${jobId}.log.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <Card>
      <CardHeader
        title={t('imports.logs.title')}
        description={t('imports.logs.count', { count: logs.length })}
        actions={
          <>
            <FilterSelect
              aria-label={t('imports.logs.level')}
              value={level}
              onChange={setLevel}
              allLabel={t('imports.logs.level.all')}
              options={[
                { value: 'info', label: t('imports.logs.level.info') },
                { value: 'warn', label: t('imports.logs.level.warn') },
                { value: 'error', label: t('imports.logs.level.error') },
              ]}
              className="sm:w-36"
            />
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Switch checked={autoScroll} onCheckedChange={setAutoScroll} aria-label={t('imports.logs.autoScroll')} />
              <span className="hidden sm:inline">{t('imports.logs.autoScroll')}</span>
            </label>
            <CopyButton value={text} label={t('imports.logs.title')} variant="secondary" size="sm" showText />
            <Button size="sm" onClick={download} disabled={logs.length === 0} icon={<Download className="size-3.5" />}>
              <span className="hidden sm:inline">{t('imports.logs.download')}</span>
              <span className="sm:hidden">.txt</span>
            </Button>
          </>
        }
      />
      {error && logs.length === 0 ? (
        <p role="alert" className="px-4 py-3 text-xs text-danger-text">
          {t('imports.logs.loadFailed')}
        </p>
      ) : logs.length === 0 ? (
        <EmptyState title={t('imports.logs.empty')} className="py-10" />
      ) : rendered.length === 0 ? (
        <EmptyState title={t('imports.logs.filteredEmpty')} className="py-10" />
      ) : (
        <>
          {filtered.length > rendered.length && (
            <p className="border-b border-border bg-muted/60 px-4 py-1.5 text-xs text-muted-foreground">
              {t('imports.logs.truncated', { shown: rendered.length, total: filtered.length })}
            </p>
          )}
          <div ref={boxRef} onScroll={onScroll} tabIndex={0} role="log" aria-live="off" className="max-h-96 overflow-auto bg-muted/40 px-4 py-2 font-mono text-xs leading-relaxed">
            {rendered.map((l) => {
              const lv = levelOf(l);
              return (
                <div key={l.id} className="flex gap-3 py-0.5">
                  <DateText value={l.created_at} relative className="w-24 shrink-0 text-muted-foreground" />
                  <span className={cn('w-12 shrink-0 font-semibold uppercase', levelStyle[lv])}>{lv}</span>
                  <span className={cn('min-w-0 flex-1 whitespace-pre-wrap break-words', lv === 'error' ? 'text-danger-text' : 'text-foreground')}>{l.message}</span>
                </div>
              );
            })}
          </div>
        </>
      )}
    </Card>
  );
}
