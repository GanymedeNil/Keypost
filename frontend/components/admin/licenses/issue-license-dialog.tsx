'use client';

import { useMemo, useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { adminApi } from '@/lib/api';
import { fromLocalInput } from '@/lib/format';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import type { LicenseKey, Paginated, Variant } from '@/lib/types';
import { useAction } from '@/lib/use-action';
import { useApi } from '@/lib/use-api';
import { Button, Checkbox, Field, Input, Modal, Select } from '@/components/kit';

interface VariantGroup {
  product: string;
  withLicense: boolean;
  variants: Variant[];
}

function groupVariants(variants: Variant[]): VariantGroup[] {
  const map = new Map<string, Variant[]>();
  for (const v of variants) map.set(v.product_name, [...(map.get(v.product_name) ?? []), v]);
  const groups = [...map.entries()].map(([product, vs]) => ({
    product,
    withLicense: vs.some((v) => v.has_license_keys),
    variants: [...vs].sort((a, b) => Number(b.has_license_keys) - Number(a.has_license_keys)),
  }));
  return groups.sort((a, b) => Number(b.withLicense) - Number(a.withLicense) || a.product.localeCompare(b.product));
}

/** Mount only while open so the form state resets each time. */
export function IssueLicenseDialog({ defaultCustomerId, onClose }: { defaultCustomerId?: string; onClose: () => void }) {
  const { t } = useI18n();
  const router = useRouter();
  const { run, pending, fieldErrors } = useAction();
  const { data: variantsData, isLoading: loadingVariants } = useApi<Paginated<Variant>>('/variants', { per_page: 100 });
  const groups = useMemo(() => groupVariants(variantsData?.data ?? []), [variantsData]);

  const [variantId, setVariantId] = useState('');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [customerId, setCustomerId] = useState(defaultCustomerId ?? '');
  const [limit, setLimit] = useState('');
  const [unlimited, setUnlimited] = useState(false);
  const [expires, setExpires] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});

  async function submit(e: FormEvent) {
    e.preventDefault();
    const local: Record<string, string> = {};
    if (!variantId) local.variant_id = t('licenses.issue.variantRequired');
    if (!email.trim() && !customerId.trim()) local.user_email = t('licenses.issue.emailRequired');
    const limitNum = Number(limit);
    if (!unlimited && limit.trim() !== '' && (!Number.isInteger(limitNum) || limitNum < 1)) local.activation_limit = t('licenses.issue.limitInvalid');
    setErrors(local);
    if (Object.keys(local).length > 0) return;

    const body: Record<string, unknown> = { variant_id: Number(variantId) };
    if (email.trim()) body.user_email = email.trim();
    if (name.trim()) body.user_name = name.trim();
    if (customerId.trim()) body.customer_id = Number(customerId);
    if (unlimited) body.activation_limit = null;
    else if (limit.trim() !== '') body.activation_limit = limitNum;
    const expiresAt = fromLocalInput(expires);
    if (expiresAt) body.expires_at = expiresAt;

    const res = await run(() => adminApi.post<LicenseKey>('/license-keys', body), { success: t('licenses.issue.toast') });
    if (res.ok) {
      await invalidate('/license-keys');
      router.push(`/admin/license-keys/${res.data.id}`);
    }
  }

  const err = (k: string) => errors[k] ?? fieldErrors[k];

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('licenses.issue.title')}
      description={t('licenses.issue.description')}
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="issue-license-form" variant="primary" loading={pending}>
            {t('licenses.issue.submit')}
          </Button>
        </>
      }
    >
      <form id="issue-license-form" onSubmit={submit} noValidate className="space-y-4">
        <Field label={t('licenses.issue.variant')} error={err('variant_id')} required>
          <Select value={variantId} onChange={(e) => setVariantId(e.target.value)}>
            <option value="">{loadingVariants ? t('licenses.issue.loadingVariants') : t('licenses.issue.variantPlaceholder')}</option>
            {groups.map((g) => (
              <optgroup key={g.product} label={g.product}>
                {g.variants.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.has_license_keys ? v.name : `${v.name} (${t('licenses.issue.noLicenseSetting')})`}
                  </option>
                ))}
              </optgroup>
            ))}
          </Select>
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('licenses.issue.email')} error={err('user_email')} required>
            <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
          </Field>
          <Field label={t('licenses.issue.name')} error={err('user_name')} optional>
            <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
          </Field>
        </div>
        <Field label={t('licenses.issue.customerId')} hint={t('licenses.issue.customerIdHint')} error={err('customer_id')} optional>
          <Input type="number" min={1} value={customerId} onChange={(e) => setCustomerId(e.target.value)} className="sm:w-40" />
        </Field>
        <div className="space-y-2">
          <Field label={t('licenses.issue.limit')} hint={t('licenses.issue.limitHint')} error={err('activation_limit')}>
            <Input type="number" min={1} step={1} value={limit} disabled={unlimited} onChange={(e) => setLimit(e.target.value)} className="sm:w-40" />
          </Field>
          <Checkbox checked={unlimited} onCheckedChange={setUnlimited} label={t('licenses.issue.unlimited')} />
        </div>
        <Field label={t('licenses.issue.expires')} hint={t('licenses.issue.expiresHint')} error={err('expires_at')} optional>
          <Input type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} className="sm:w-60" />
        </Field>
      </form>
    </Modal>
  );
}
