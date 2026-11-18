'use client';

import { useState, type FormEvent } from 'react';
import { Trash2 } from 'lucide-react';
import { adminApi, ApiError } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { errorText } from '@/lib/error-text';
import { useCan } from '@/components/admin/admin-context';
import type { Variant, VariantLink } from '@/lib/types';
import { Button, Field, Input, Select, Textarea, useConfirmDelete, useToast } from '@/components/kit';
import { LinkEditor } from './link-editor';
import { VARIANT_STATUSES } from './price-utils';

interface Draft {
  name: string;
  slug: string;
  description: string;
  status: string;
  sort: string;
  links: VariantLink[];
}

function toDraft(v: Variant): Draft {
  return {
    name: v.name,
    slug: v.slug,
    description: v.description ?? '',
    status: v.status,
    sort: String(v.sort),
    links: (v.links ?? []).map((l) => ({ title: l.title, url: l.url })),
  };
}

export function DetailsTab({ variant, onChanged, onDeleted }: { variant: Variant; onChanged: () => Promise<unknown>; onDeleted: () => Promise<unknown> }) {
  const { t, te } = useI18n();
  const can = useCan();
  const toast = useToast();
  const confirmDelete = useConfirmDelete();
  const [draft, setDraft] = useState<Draft>(() => toDraft(variant));
  const { run, pending, fieldErrors } = useAction();
  const del = useAction();
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => ({ ...d, [k]: v }));

  const dirty = JSON.stringify(draft) !== JSON.stringify(toDraft(variant));

  async function save(e: FormEvent) {
    e.preventDefault();
    const sort = Number(draft.sort);
    const res = await run(
      () =>
        adminApi.patch<Variant>(`/variants/${variant.id}`, {
          name: draft.name.trim(),
          slug: draft.slug.trim(),
          description: draft.description,
          status: draft.status,
          sort: Number.isFinite(sort) ? sort : variant.sort,
          links: draft.links.filter((l) => l.title.trim() || l.url.trim()).map((l) => ({ title: l.title.trim(), url: l.url.trim() })),
        }),
      { success: t('toast.saved') },
    );
    if (res.ok) await onChanged();
  }

  async function remove() {
    if (!(await confirmDelete(variant.name, t('products.variant.deleteBody')))) return;
    const res = await del.run(() => adminApi.delete(`/variants/${variant.id}`), { success: t('toast.deleted'), silent: true });
    if (res.ok) await onDeleted();
    else toast.error(res.error instanceof ApiError && res.error.code === 'conflict' ? t('products.variant.deleteConflict') : errorText(res.error, t));
  }

  return (
    <form onSubmit={save} noValidate>
      <fieldset disabled={!can.write} className="min-w-0 space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('common.name')} required error={fieldErrors.name}>
            <Input value={draft.name} onChange={(e) => set('name', e.target.value)} />
          </Field>
          <Field label={t('common.slug')} error={fieldErrors.slug}>
            <Input value={draft.slug} onChange={(e) => set('slug', e.target.value)} />
          </Field>
        </div>
        <Field label={t('common.description')} optional error={fieldErrors.description}>
          <Textarea value={draft.description} onChange={(e) => set('description', e.target.value)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('common.status')} error={fieldErrors.status}>
            <Select value={draft.status} onChange={(e) => set('status', e.target.value)}>
              {VARIANT_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {te(`status.${s}`, s)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('common.sort')} hint={t('products.variant.sortHint')} error={fieldErrors.sort}>
            <Input type="number" step={1} value={draft.sort} onChange={(e) => set('sort', e.target.value)} />
          </Field>
        </div>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-foreground">{t('products.links.title')}</p>
          <LinkEditor links={draft.links} onChange={(links) => set('links', links)} />
          {fieldErrors.links ? (
            <p role="alert" className="text-xs text-danger-text">
              {fieldErrors.links}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">{t('products.links.hint')}</p>
          )}
        </div>
      </fieldset>

      {can.write && (
        <div className="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
          <Button variant="danger-outline" icon={<Trash2 className="size-3.5" />} loading={del.pending} onClick={remove}>
            {t('products.variant.delete')}
          </Button>
          <Button type="submit" variant="primary" loading={pending} disabled={!dirty}>
            {t('common.saveChanges')}
          </Button>
        </div>
      )}
    </form>
  );
}
