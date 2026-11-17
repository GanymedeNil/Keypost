'use client';

import { Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import useSWR, { mutate } from 'swr';
import { CheckCircle2, LinkIcon } from 'lucide-react';
import { ApiError, storefrontApi } from '@/lib/api';
import { errorText } from '@/lib/error-text';
import { useI18n } from '@/lib/i18n/provider';
import { ME_KEY } from '@/lib/storefront';
import { ButtonLink, Card, CardBody, Spinner, usePageTitle } from '@/components/ui';

function Notice({ icon, tone, title, body, children }: { icon: React.ReactNode; tone: 'success' | 'danger'; title: string; body?: string; children?: React.ReactNode }) {
  return (
    <Card className="mx-auto max-w-md">
      <CardBody className="flex flex-col items-center gap-3 px-6 py-10 text-center">
        <span className={tone === 'success' ? 'grid size-12 place-items-center rounded-full bg-success-soft text-success-text' : 'grid size-12 place-items-center rounded-full bg-danger-soft text-danger-text'}>
          {icon}
        </span>
        <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
        {body && <p className="text-sm text-muted">{body}</p>}
        {children}
      </CardBody>
    </Card>
  );
}

function VerifyInner() {
  const { t } = useI18n();
  const router = useRouter();
  const token = useSearchParams().get('token')?.trim() || null;

  // One-time token: the SWR key dedupes the request (React strict mode / re-renders never post twice).
  const { data, error } = useSWR(
    token ? (['storefront-verify', token] as const) : null,
    ([, tk]) => storefrontApi.post<{ email: string }>('/auth/verify', { token: tk }),
    {
      revalidateOnFocus: false,
      revalidateOnReconnect: false,
      revalidateIfStale: false,
      shouldRetryOnError: false,
      onSuccess: async () => {
        await mutate(ME_KEY);
        router.replace('/my-orders');
      },
    },
  );

  const invalid = !token || (error instanceof ApiError && (error.code === 'invalid_token' || error.status === 400));
  usePageTitle(invalid || error ? t('storefront.verify.invalidTitle') : t('storefront.verify.signingIn'));
  const requestNew = (
    <ButtonLink href="/my-orders" variant="primary" size="lg" className="mt-2 h-11 w-full sm:w-auto">
      {t('storefront.verify.requestNew')}
    </ButtonLink>
  );

  if (invalid) {
    return (
      <Notice
        tone="danger"
        icon={<LinkIcon className="size-6" aria-hidden />}
        title={t('storefront.verify.invalidTitle')}
        body={t('storefront.verify.invalidBody')}
      >
        {requestNew}
      </Notice>
    );
  }
  if (error) {
    return (
      <Notice tone="danger" icon={<LinkIcon className="size-6" aria-hidden />} title={t('storefront.verify.errorTitle')} body={errorText(error, t)}>
        {requestNew}
      </Notice>
    );
  }
  if (data) {
    return (
      <Notice tone="success" icon={<CheckCircle2 className="size-6" aria-hidden />} title={t('storefront.verify.success')}>
        <ButtonLink href="/my-orders" variant="primary" size="lg" className="mt-2 h-11 w-full sm:w-auto">
          {t('storefront.verify.continue')}
        </ButtonLink>
      </Notice>
    );
  }
  return (
    <Card className="mx-auto max-w-md">
      <CardBody className="flex flex-col items-center gap-3 px-6 py-12 text-center" role="status">
        <Spinner className="size-7 text-accent" />
        <h1 className="text-lg font-semibold tracking-tight">{t('storefront.verify.signingIn')}</h1>
        <p className="text-sm text-muted">{t('storefront.verify.signingInHint')}</p>
      </CardBody>
    </Card>
  );
}

export default function VerifyPage() {
  return (
    <Suspense>
      <VerifyInner />
    </Suspense>
  );
}
