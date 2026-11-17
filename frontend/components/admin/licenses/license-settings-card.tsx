'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { fromLocalInput, toLocalInput } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { LicenseKeyDetail } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import { Button, Card, CardBody, CardFooter, CardHeader, Field, Input, Progress, Switch, SwitchField } from '@/components/ui';

/** Remount with `key` (e.g. `updated_at`) after a save so the form re-reads the server values. */
export function LicenseSettingsCard({ lk, onSaved }: { lk: LicenseKeyDetail; onSaved: () => Promise<unknown> }) {
  const { t } = useI18n();
  const can = useCan();
  const { run, pending, fieldErrors } = useAction();

  const [unlimited, setUnlimited] = useState(lk.activation_limit === null);
  const [limit, setLimit] = useState(lk.activation_limit === null ? '' : String(lk.activation_limit));
  const [expires, setExpires] = useState(toLocalInput(lk.expires_at));
  const [disabled, setDisabled] = useState(lk.disabled);
  const [limitError, setLimitError] = useState<string | null>(null);

  const used = lk.instances_count;
  const usageLimit = lk.activation_limit;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const n = Number(limit);
    if (!unlimited && (limit.trim() === '' || !Number.isInteger(n) || n < 1)) return setLimitError(t('licenses.settings.limitInvalid'));
    setLimitError(null);
    const res = await run(
      () => adminApi.patch(`/license-keys/${lk.id}`, { activation_limit: unlimited ? null : n, expires_at: fromLocalInput(expires), disabled }),
      { success: t('licenses.settings.saved') },
    );
    if (res.ok) await Promise.all([onSaved(), invalidate('/license-keys')]);
  }

  return (
    <Card>
      <CardHeader title={t('licenses.settings.title')} />
      <form onSubmit={submit} noValidate>
        <CardBody className="space-y-5">
          <div className="space-y-2">
            <div className="flex items-baseline justify-between gap-2 text-xs text-muted">
              <span>
                {usageLimit === null ? t('licenses.settings.usageUnlimited', { used }) : t('licenses.settings.usage', { used, limit: usageLimit })}
              </span>
            </div>
            {usageLimit !== null && (
              <Progress value={used} max={usageLimit} tone={used >= usageLimit ? 'warning' : 'accent'} label={t('licenses.settings.limit')} />
            )}
          </div>

          <div className="space-y-2.5">
            <Field label={t('licenses.settings.limit')} error={limitError ?? fieldErrors.activation_limit}>
              <Input
                type="number"
                min={1}
                step={1}
                value={unlimited ? '' : limit}
                placeholder={unlimited ? '∞' : undefined}
                disabled={unlimited || !can.write}
                onChange={(e) => setLimit(e.target.value)}
              />
            </Field>
            <SwitchField
              label={t('licenses.settings.unlimited')}
              description={t('licenses.settings.unlimitedHint')}
              checked={unlimited}
              onCheckedChange={setUnlimited}
              disabled={!can.write}
            />
          </div>

          <Field label={t('licenses.settings.expires')} hint={t('licenses.settings.expiresHint')} error={fieldErrors.expires_at}>
            <div className="flex items-center gap-2">
              <Input type="datetime-local" value={expires} disabled={!can.write} onChange={(e) => setExpires(e.target.value)} />
              {can.write && expires && (
                <Button size="sm" variant="ghost" onClick={() => setExpires('')}>
                  {t('licenses.settings.clearExpiry')}
                </Button>
              )}
            </div>
          </Field>

          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <p className="text-sm font-medium text-fg">{t('licenses.settings.disabled')}</p>
              <p className="mt-0.5 text-xs text-subtle">{t('licenses.settings.disabledHint')}</p>
            </div>
            <Switch checked={disabled} onCheckedChange={setDisabled} disabled={!can.write} aria-label={t('licenses.settings.disabled')} />
          </div>
        </CardBody>
        {can.write && (
          <CardFooter>
            <Button type="submit" variant="primary" loading={pending}>
              {t('common.saveChanges')}
            </Button>
          </CardFooter>
        )}
      </form>
    </Card>
  );
}
