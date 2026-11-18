'use client';

import { useState } from 'react';
import { CheckCircle2, CloudUpload } from 'lucide-react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import type { StripeStatus } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import {
  Badge,
  Button,
  ButtonLink,
  Callout,
  Card,
  CardBody,
  CardHeader,
  CopyField,
  DI,
  DL,
  ErrorState,
  Skeleton,
  SkeletonText,
} from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';

interface WebhookEndpointResult {
  id: string;
  url: string;
  events: string[];
  created: boolean;
}

export function StatusTab() {
  const { t } = useI18n();
  const can = useCan();
  const { data, error, isLoading, mutate } = useApi<StripeStatus>('/stripe/status', undefined, { scoped: false });
  const { run, pending } = useAction();
  const [result, setResult] = useState<WebhookEndpointResult | null>(null);

  async function syncEndpoint() {
    const res = await run(() => adminApi.post<WebhookEndpointResult>('/stripe/webhook-endpoint'));
    if (res.ok) {
      setResult(res.data);
      await mutate();
    }
  }

  if (error) return <Card><ErrorState error={error} onRetry={() => mutate()} /></Card>;
  if (isLoading || !data) {
    return (
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardBody className="space-y-3">
            <Skeleton className="h-5 w-32" />
            <SkeletonText lines={5} />
          </CardBody>
        </Card>
        <Card>
          <CardBody className="space-y-3">
            <Skeleton className="h-5 w-40" />
            <SkeletonText lines={4} />
          </CardBody>
        </Card>
      </div>
    );
  }

  const account = data.account;
  return (
    <div className="space-y-4">
      {!data.configured && (
        <Callout
          tone="warning"
          title={t('stripe.notConfigured.title')}
          action={
            <ButtonLink href="/admin/settings/stripe" size="sm" variant="secondary">
              {t('stripe.notConfigured.action')}
            </ButtonLink>
          }
        >
          {t('stripe.notConfigured.body')}
        </Callout>
      )}
      {data.configured && !data.webhook_secret_set && (
        <Callout
          tone="warning"
          title={t('stripe.secretMissing.title')}
          action={
            <ButtonLink href="/admin/settings/stripe" size="sm" variant="secondary">
              {t('stripe.notConfigured.action')}
            </ButtonLink>
          }
        >
          {t('stripe.secretMissing.body')}
        </Callout>
      )}
      {data.error && (
        <Callout tone="danger" title={t('stripe.status.problem')}>
          {data.error}
        </Callout>
      )}

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader
            title={t('stripe.status.title')}
            actions={
              <Badge dot tone={data.configured ? 'success' : 'warning'}>
                {data.configured ? t('stripe.status.connected') : t('stripe.status.notConfigured')}
              </Badge>
            }
          />
          <CardBody>
            <DL cols={2}>
              <DI label={t('stripe.status.mode')}>
                {data.configured ? <Badge tone={data.livemode ? 'success' : 'warning'}>{data.livemode ? t('stripe.status.liveMode') : t('stripe.status.testMode')}</Badge> : null}
              </DI>
              <DI label={t('stripe.status.webhookSecret')}>
                <Badge dot tone={data.webhook_secret_set ? 'success' : 'danger'}>
                  {data.webhook_secret_set ? t('stripe.status.secretSet') : t('stripe.status.secretMissing')}
                </Badge>
              </DI>
              <DI label={t('stripe.status.accountId')}>{account?.id ? <code className="break-all text-xs">{account.id}</code> : null}</DI>
              <DI label={t('stripe.status.businessName')}>{account?.business_name}</DI>
              <DI label={t('stripe.status.email')}>{account?.email}</DI>
              <DI label={t('stripe.status.country')}>{account?.country}</DI>
            </DL>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title={t('stripe.webhook.title')} description={t('stripe.webhook.description')} />
          <CardBody className="space-y-4">
            <div>
              <p className="mb-1.5 text-xs font-medium text-foreground">{t('stripe.webhook.url')}</p>
              <CopyField value={data.webhook_url} label={t('stripe.webhook.url')} />
            </div>
            <div>
              <p className="mb-1.5 text-xs font-medium text-foreground">
                {t('stripe.webhook.requiredEvents')} <span className="font-normal text-muted-foreground">({data.required_events.length})</span>
              </p>
              <ul className="flex flex-wrap gap-1.5">
                {data.required_events.map((e) => (
                  <li key={e} className="rounded-md bg-accent px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                    {e}
                  </li>
                ))}
              </ul>
            </div>
            {result && (
              <Callout tone="success" title={result.created ? t('stripe.webhook.created') : t('stripe.webhook.updated')}>
                <p>{t('stripe.webhook.resultBody', { url: result.url, count: result.events.length })}</p>
                <p className="mt-0.5 font-mono text-xs opacity-80">{t('stripe.webhook.endpointId', { id: result.id })}</p>
              </Callout>
            )}
            {can.write && (
              <Button
                variant="primary"
                loading={pending}
                disabled={!data.configured}
                icon={result ? <CheckCircle2 className="size-4" /> : <CloudUpload className="size-4" />}
                onClick={syncEndpoint}
              >
                {pending ? t('stripe.webhook.working') : t('stripe.webhook.submit')}
              </Button>
            )}
          </CardBody>
        </Card>
      </div>
    </div>
  );
}
