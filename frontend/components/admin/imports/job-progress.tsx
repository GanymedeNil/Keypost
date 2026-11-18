'use client';

import { Check, Circle, Loader2, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { formatNumber } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import type { ImportJob } from '@/lib/types';
import { Card, CardBody, CardHeader, Progress } from '@/components/kit';
import { isActive, isLemonSqueezyKind, LS_STAGES, progressTone, useImportLabels } from './utils';

type StepState = 'done' | 'current' | 'upcoming' | 'stopped';

/** The ordered stage list for this job: the selected LS resources, or the stages seen so far for other kinds. */
function stageList(job: ImportJob): string[] {
  const current = job.progress?.stage;
  if (isLemonSqueezyKind(job.kind)) {
    const selected = Array.isArray(job.options?.resources) ? (job.options.resources as string[]) : null;
    const base = LS_STAGES.filter((s) => !selected || selected.includes(s));
    return current && !(base as readonly string[]).includes(current) ? [...base, current] : [...base];
  }
  const seen = Object.keys(job.progress?.counts ?? {});
  return current && !seen.includes(current) ? [...seen, current] : seen;
}

function stepState(job: ImportJob, index: number, currentIndex: number): StepState {
  if (job.status === 'completed') return 'done';
  if (index < currentIndex) return 'done';
  if (index === currentIndex) return job.status === 'failed' || job.status === 'cancelled' ? 'stopped' : 'current';
  return 'upcoming';
}

const stateIcon: Record<StepState, React.ReactNode> = {
  done: <Check className="size-3.5" />,
  current: <Loader2 className="size-3.5 animate-spin-slow" />,
  upcoming: <Circle className="size-3 opacity-50" />,
  stopped: <X className="size-3.5" />,
};

const stateStyle: Record<StepState, string> = {
  done: 'border-success/30 bg-success-soft text-success-text',
  current: 'border-primary/40 bg-accent text-primary',
  upcoming: 'border-border bg-card text-muted-foreground',
  stopped: 'border-danger/30 bg-danger-soft text-danger-text',
};

export function JobProgress({ job }: { job: ImportJob }) {
  const { t, locale } = useI18n();
  const labels = useImportLabels();
  const p = job.progress;
  const stages = stageList(job);
  const currentIndex = p ? stages.indexOf(p.stage) : -1;
  const finished = job.status === 'completed';
  const stageValue = finished ? 1 : p && p.total > 0 ? p.done / p.total : 0;
  const overall = finished ? 1 : stages.length > 0 && currentIndex >= 0 ? (currentIndex + stageValue) / stages.length : 0;
  const counts = Object.entries(p?.counts ?? {});

  return (
    <Card>
      <CardHeader
        title={t('imports.progress.title')}
        actions={isActive(job.status) && <span className="text-xs text-muted-foreground">{t('imports.job.live')}</span>}
      />
      <CardBody className="space-y-5">
        {!p ? (
          <p className="text-sm text-muted-foreground">{t('imports.job.notStarted')}</p>
        ) : (
          <>
            <div className="space-y-4">
              <div className="space-y-1.5">
                <div className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="text-muted-foreground">
                    {t('imports.progress.stage')}: <span className="font-medium text-foreground">{labels.stage(p.stage)}</span>
                  </span>
                  {p.total > 0 && (
                    <span className="tabular-nums text-muted-foreground">
                      {t('imports.progress.items', { done: formatNumber(finished ? p.total : p.done, locale), total: formatNumber(p.total, locale) })}
                    </span>
                  )}
                </div>
                <Progress value={stageValue} max={1} tone={progressTone(job.status)} label={labels.stage(p.stage)} />
              </div>
              {stages.length > 1 && (
                <div className="space-y-1.5">
                  <div className="flex items-baseline justify-between text-xs text-muted-foreground">
                    <span>{t('imports.progress.overall')}</span>
                    <span className="tabular-nums">{Math.round(overall * 100)}%</span>
                  </div>
                  <Progress value={overall} max={1} tone={progressTone(job.status)} className="h-1" label={t('imports.progress.overall')} />
                </div>
              )}
            </div>

            {stages.length > 0 && (
              <ol className="flex flex-wrap gap-2" aria-label={t('imports.progress.title')}>
                {stages.map((s, i) => {
                  const state = stepState(job, i, currentIndex);
                  return (
                    <li
                      key={s}
                      aria-current={state === 'current' ? 'step' : undefined}
                      className={cn('inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium', stateStyle[state])}
                    >
                      {stateIcon[state]}
                      {labels.stage(s)}
                      <span className="sr-only">: {t(`imports.stepper.${state === 'stopped' ? 'failed' : state}`)}</span>
                    </li>
                  );
                })}
              </ol>
            )}

            <div>
              <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t('imports.counts.title')}</h3>
              <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-5">
                {counts.map(([k, n]) => (
                  <div key={k} className="rounded-lg border border-border bg-muted/50 px-3 py-2">
                    <p className="truncate text-xs text-muted-foreground">{labels.stage(k)}</p>
                    <p className="text-lg font-semibold tabular-nums">{formatNumber(n, locale)}</p>
                  </div>
                ))}
                <div className={cn('rounded-lg border px-3 py-2', p.warnings > 0 ? 'border-warning/30 bg-warning-soft' : 'border-border bg-muted/50')}>
                  <p className={cn('truncate text-xs', p.warnings > 0 ? 'text-warning-text' : 'text-muted-foreground')}>{t('imports.warnings')}</p>
                  <p className={cn('text-lg font-semibold tabular-nums', p.warnings > 0 && 'text-warning-text')}>{formatNumber(p.warnings, locale)}</p>
                </div>
              </div>
              {counts.length === 0 && <p className="mt-2 text-xs text-muted-foreground">{t('imports.counts.empty')}</p>}
            </div>
          </>
        )}
      </CardBody>
    </Card>
  );
}
