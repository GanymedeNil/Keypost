'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import type { ImportJob } from '@/lib/types';
import { useT } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { Button, Checkbox } from '@/components/ui';
import { StepCard } from './step-card';
import { useOpenJob } from './utils';

type Match = { customers: boolean; products: boolean; subscriptions: boolean };
const KEYS = ['customers', 'products', 'subscriptions'] as const;

export function AutoMatchStep({ disabled }: { disabled: boolean }) {
  const t = useT();
  const openJob = useOpenJob();
  const { run, pending, fieldErrors } = useAction();
  const [match, setMatch] = useState<Match>({ customers: true, products: true, subscriptions: true });
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!KEYS.some((k) => match[k])) return setError(t('imports.err.noKinds'));
    setError(null);
    const res = await run(() => adminApi.post<ImportJob>('/imports/stripe-auto-match', match), { success: t('imports.toast.started') });
    if (res.ok) openJob(res.data);
  }

  const message = error ?? Object.values(fieldErrors)[0];
  return (
    <StepCard step={3} title={t('imports.step3.title')} description={t('imports.step3.desc')} disabled={disabled}>
      <form onSubmit={submit} className="space-y-5" noValidate>
        <div className="grid gap-3 sm:grid-cols-3">
          {KEYS.map((k) => (
            <div key={k} className="rounded-lg border border-line bg-surface-2/50 p-3">
              <Checkbox
                label={t(`imports.match.${k}`)}
                description={t(`imports.match.${k}Desc`)}
                checked={match[k]}
                onCheckedChange={(on) => setMatch((m) => ({ ...m, [k]: on }))}
              />
            </div>
          ))}
        </div>
        {message && (
          <p role="alert" className="text-xs text-danger-text">
            {message}
          </p>
        )}
        <Button type="submit" variant="primary" loading={pending} disabled={disabled}>
          {t('imports.step3.start')}
        </Button>
      </form>
    </StepCard>
  );
}
