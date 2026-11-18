'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { useCan } from '@/components/admin/admin-context';
import type { LicenseLengthUnit, Variant } from '@/lib/types';
import { Button, Field, Input, Select, SwitchField } from '@/components/kit';

const UNITS: LicenseLengthUnit[] = ['days', 'months', 'years'];

interface Draft {
  enabled: boolean;
  limit: string;
  unlimitedLimit: boolean;
  length: string;
  unit: LicenseLengthUnit;
  unlimitedLength: boolean;
}

function toDraft(v: Variant): Draft {
  return {
    enabled: v.has_license_keys,
    limit: String(v.license_activation_limit ?? 1),
    unlimitedLimit: v.is_license_limit_unlimited,
    length: String(v.license_length_value ?? 1),
    unit: v.license_length_unit ?? 'years',
    unlimitedLength: v.is_license_length_unlimited,
  };
}

function positiveInt(value: string, fallback: number): number {
  const n = Math.floor(Number(value));
  return Number.isFinite(n) && n >= 1 ? n : fallback;
}

export function LicenseTab({ variant, onChanged }: { variant: Variant; onChanged: () => Promise<unknown> }) {
  const { t, te } = useI18n();
  const can = useCan();
  const [draft, setDraft] = useState<Draft>(() => toDraft(variant));
  const { run, pending, fieldErrors } = useAction();
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => ({ ...d, [k]: v }));
  const dirty = JSON.stringify(draft) !== JSON.stringify(toDraft(variant));

  async function save(e: FormEvent) {
    e.preventDefault();
    const res = await run(
      () =>
        adminApi.patch<Variant>(`/variants/${variant.id}`, {
          has_license_keys: draft.enabled,
          license_activation_limit: positiveInt(draft.limit, variant.license_activation_limit ?? 1),
          is_license_limit_unlimited: draft.unlimitedLimit,
          license_length_value: positiveInt(draft.length, variant.license_length_value ?? 1),
          license_length_unit: draft.unit,
          is_license_length_unlimited: draft.unlimitedLength,
        }),
      { success: t('toast.saved') },
    );
    if (res.ok) await onChanged();
  }

  return (
    <form onSubmit={save} noValidate>
      <fieldset disabled={!can.write} className="min-w-0 space-y-5">
        <SwitchField
          label={t('products.license.enable')}
          description={t('products.license.enableHint')}
          checked={draft.enabled}
          onCheckedChange={(v) => set('enabled', v)}
        />
        {draft.enabled && (
          <>
            <div className="space-y-3 rounded-lg border border-border p-3">
              <h3 className="text-sm font-medium">{t('products.license.activations')}</h3>
              <SwitchField
                label={t('products.license.unlimitedActivations')}
                checked={draft.unlimitedLimit}
                onCheckedChange={(v) => set('unlimitedLimit', v)}
              />
              <Field label={t('products.license.activationLimit')} hint={t('products.license.activationLimitHint')} error={fieldErrors.license_activation_limit}>
                <Input
                  type="number"
                  min={1}
                  step={1}
                  value={draft.limit}
                  disabled={draft.unlimitedLimit || !can.write}
                  onChange={(e) => set('limit', e.target.value)}
                  className="max-w-40"
                />
              </Field>
            </div>
            <div className="space-y-3 rounded-lg border border-border p-3">
              <h3 className="text-sm font-medium">{t('products.license.validity')}</h3>
              <SwitchField
                label={t('products.license.unlimitedLength')}
                description={t('products.license.unlimitedLengthHint')}
                checked={draft.unlimitedLength}
                onCheckedChange={(v) => set('unlimitedLength', v)}
              />
              <div className="grid max-w-md gap-4 sm:grid-cols-2">
                <Field label={t('products.license.length')} error={fieldErrors.license_length_value}>
                  <Input
                    type="number"
                    min={1}
                    step={1}
                    value={draft.length}
                    disabled={draft.unlimitedLength || !can.write}
                    onChange={(e) => set('length', e.target.value)}
                  />
                </Field>
                <Field label={t('products.license.unit')} error={fieldErrors.license_length_unit}>
                  <Select
                    value={draft.unit}
                    disabled={draft.unlimitedLength || !can.write}
                    onChange={(e) => set('unit', e.target.value as LicenseLengthUnit)}
                  >
                    {UNITS.map((u) => (
                      <option key={u} value={u}>
                        {te(`products.licenseUnit.${u}`, u)}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
            </div>
          </>
        )}
      </fieldset>
      {can.write && (
        <div className="mt-5 flex justify-end border-t border-border pt-4">
          <Button type="submit" variant="primary" loading={pending} disabled={!dirty}>
            {t('common.saveChanges')}
          </Button>
        </div>
      )}
    </form>
  );
}
