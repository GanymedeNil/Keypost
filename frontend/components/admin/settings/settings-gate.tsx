'use client';

import { useCallback, useState, type ReactNode } from 'react';
import { Lock } from 'lucide-react';
import { adminApi, isApiError } from '@/lib/api';
import { useT } from '@/lib/i18n/provider';
import type { Settings, SettingsUpdate } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { Callout, Card, CardBody, EmptyState, ErrorState, Skeleton } from '@/components/ui';
import { useAdmin } from '../admin-context';

export interface SettingsCtx {
  settings: Settings;
  /** Only owners may `PUT /settings`; admins see the forms read-only. */
  readOnly: boolean;
  pending: boolean;
  /** Validation errors keyed like the API does (`smtp.port`, `stripe.login_mode`, …). */
  fieldErrors: Record<string, string>;
  /** PUT a partial update; resolves true on success (the form is re-initialised from the saved data). */
  save: (update: SettingsUpdate) => Promise<boolean>;
}

function SettingsSkeleton() {
  return (
    <div className="space-y-6" role="status" aria-busy="true">
      {[0, 1].map((i) => (
        <Card key={i}>
          <CardBody className="space-y-4">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </CardBody>
        </Card>
      ))}
    </div>
  );
}

/**
 * Loads `GET /settings` and hands it to `children`. Saving re-keys the rendered form so that its draft
 * state is re-initialised from the freshly saved values (no setState-in-effect needed).
 */
export function SettingsGate({ children }: { children: (ctx: SettingsCtx) => ReactNode }) {
  const t = useT();
  const { can } = useAdmin();
  const { data, error, mutate } = useApi<Settings>('/settings', undefined, { scoped: false, revalidateOnFocus: false });
  const { run, pending, fieldErrors } = useAction();
  const [version, setVersion] = useState(0);

  const save = useCallback(
    async (update: SettingsUpdate) => {
      const res = await run(() => adminApi.put<Settings>('/settings', update), { success: t('toast.saved') });
      if (!res.ok) return false;
      if (res.data) await mutate(res.data, { revalidate: false });
      else await mutate();
      setVersion((v) => v + 1);
      return true;
    },
    [run, t, mutate],
  );

  if (error && isApiError(error) && error.status === 403) {
    return <EmptyState icon={<Lock className="size-5" />} title={t('settings.forbidden.title')} description={t('settings.forbidden.body')} />;
  }
  if (error) return <ErrorState error={error} onRetry={() => mutate()} />;
  if (!data) return <SettingsSkeleton />;

  return (
    <div key={version} className="space-y-6">
      {!can.owner && <Callout tone="info">{t('settings.ownerOnly')}</Callout>}
      {children({ settings: data, readOnly: !can.owner, pending, fieldErrors, save })}
    </div>
  );
}
