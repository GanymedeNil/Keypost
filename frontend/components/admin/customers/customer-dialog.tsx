'use client';

import { useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { invalidate } from '@/lib/swr';
import { useAction } from '@/lib/use-action';
import type { Customer } from '@/lib/types';
import { Button, Field, Input, Modal } from '@/components/ui';
import { StoreSelect, useDefaultStoreId } from '../store-select';
import { useAdmin } from '../admin-context';

/** Mount only while open (fresh state on every open). */
export function CreateCustomerDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (customer: Customer) => void }) {
  const { t } = useI18n();
  const { testMode } = useAdmin();
  const defaultStore = useDefaultStoreId();
  const { run, pending, fieldErrors } = useAction();
  const [storeId, setStoreId] = useState<number | undefined>(defaultStore);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');

  async function submit(e: FormEvent) {
    e.preventDefault();
    const res = await run(
      () => adminApi.post<Customer>('/customers', { store_id: storeId, name: name.trim(), email: email.trim(), test_mode: testMode }),
      { success: t('customers.create.success') },
    );
    if (res.ok) {
      await invalidate('/customers');
      onCreated(res.data);
      onClose();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('customers.create.title')}
      description={t('customers.create.description')}
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="customer-create-form" variant="primary" loading={pending}>
            {t('customers.create.submit')}
          </Button>
        </>
      }
    >
      <form id="customer-create-form" onSubmit={submit} className="space-y-4" noValidate>
        <Field label={t('common.store')} error={fieldErrors.store_id} required>
          <StoreSelect value={storeId} onChange={setStoreId} />
        </Field>
        <Field label={t('customers.field.name')} error={fieldErrors.name} required>
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus autoComplete="off" />
        </Field>
        <Field label={t('customers.field.email')} error={fieldErrors.email} required>
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
        </Field>
      </form>
    </Modal>
  );
}

/** Mount only while open (fresh state on every open). */
export function EditCustomerDialog({ customer, onClose, onSaved }: { customer: Customer; onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n();
  const { run, pending, fieldErrors } = useAction();
  const [form, setForm] = useState({
    name: customer.name ?? '',
    email: customer.email ?? '',
    city: customer.city ?? '',
    region: customer.region ?? '',
    country: customer.country ?? '',
    stripe_customer_id: customer.stripe_customer_id ?? '',
  });
  const bind = (key: keyof typeof form) => ({
    value: form[key],
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [key]: e.target.value })),
  });

  async function submit(e: FormEvent) {
    e.preventDefault();
    const body = {
      name: form.name.trim(),
      email: form.email.trim(),
      city: form.city.trim(),
      region: form.region.trim(),
      country: form.country.trim().toUpperCase(),
      stripe_customer_id: form.stripe_customer_id.trim() || null,
    };
    const res = await run(() => adminApi.patch<Customer>(`/customers/${customer.id}`, body), { success: t('toast.saved') });
    if (res.ok) {
      await invalidate('/customers');
      onSaved();
      onClose();
    }
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('customers.edit.title')}
      dismissible={!pending}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="customer-edit-form" variant="primary" loading={pending}>
            {t('common.saveChanges')}
          </Button>
        </>
      }
    >
      <form id="customer-edit-form" onSubmit={submit} className="space-y-4" noValidate>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('customers.field.name')} error={fieldErrors.name} required>
            <Input {...bind('name')} autoComplete="off" />
          </Field>
          <Field label={t('customers.field.email')} error={fieldErrors.email} required>
            <Input type="email" {...bind('email')} autoComplete="off" />
          </Field>
          <Field label={t('customers.field.city')} error={fieldErrors.city}>
            <Input {...bind('city')} autoComplete="off" />
          </Field>
          <Field label={t('customers.field.region')} error={fieldErrors.region}>
            <Input {...bind('region')} autoComplete="off" />
          </Field>
          <Field label={t('customers.field.country')} hint={t('customers.field.countryHint')} error={fieldErrors.country}>
            <Input {...bind('country')} maxLength={2} className="uppercase" autoComplete="off" />
          </Field>
        </div>
        <Field label={t('customers.field.stripeId')} hint={t('customers.field.stripeIdHint')} error={fieldErrors.stripe_customer_id} optional>
          <Input {...bind('stripe_customer_id')} placeholder="cus_…" className="font-mono" autoComplete="off" spellCheck={false} />
        </Field>
      </form>
    </Modal>
  );
}
