'use client';

import { useId, useState, type FormEvent } from 'react';
import { adminApi } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import type { Role, User } from '@/lib/types';
import { Button, Field, Input, Modal, Select, SwitchField } from '@/components/kit';
import { useFieldErrors } from './use-field-errors';

const ROLES: Role[] = ['owner', 'admin', 'viewer'];

/** Add (no `user`) or edit (`user`) a team member. Mount it only while open so the form resets each time. */
export function UserDialog({ user, onClose, onSaved }: { user?: User; onClose: () => void; onSaved: () => void }) {
  const { t, te } = useI18n();
  const formId = useId();
  const editing = !!user;
  const [email, setEmail] = useState(user?.email ?? '');
  const [name, setName] = useState(user?.name ?? '');
  const [role, setRole] = useState<Role>(user?.role ?? 'viewer');
  const [password, setPassword] = useState('');
  const [stripe, setStripe] = useState(user?.stripe_account_id ?? '');
  const [disabled, setDisabled] = useState(user?.disabled ?? false);
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({});
  const { run, pending, fieldErrors } = useAction();
  const { errors, touch, reset } = useFieldErrors(fieldErrors, localErrors);

  async function submit(e: FormEvent) {
    e.preventDefault();
    reset();
    if (password && password.length < 10) {
      setLocalErrors({ password: t('team.users.passwordShort') });
      return;
    }
    setLocalErrors({});
    const acct = stripe.trim();
    let body: Record<string, unknown>;
    if (user) {
      body = { name: name.trim(), role, disabled };
      if (password) body.password = password;
      // Only touch the Stripe link when it changed ("" clears it).
      if (acct !== (user.stripe_account_id ?? '')) body.stripe_account_id = acct;
    } else {
      body = { email: email.trim(), name: name.trim(), role };
      if (password) body.password = password;
      if (acct) body.stripe_account_id = acct;
    }
    const res = await run(
      () => (user ? adminApi.patch<User>(`/users/${user.id}`, body) : adminApi.post<User>('/users', body)),
      { success: user ? t('team.users.updated') : t('team.users.created') },
    );
    if (res.ok) onSaved();
  }

  return (
    <Modal
      open
      onOpenChange={(o) => !o && !pending && onClose()}
      title={user ? t('team.users.editTitle', { name: user.name || user.email }) : t('team.users.add')}
      description={user ? user.email : t('team.users.addDesc')}
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending} disabled={!name.trim() || (!editing && !email.trim())}>
            {editing ? t('common.save') : t('team.users.add')}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} className="space-y-4" noValidate>
        {!editing && (
          <Field label={t('common.email')} error={errors.email} required>
            <Input
              type="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                touch('email');
              }}
              autoComplete="off"
              autoFocus
            />
          </Field>
        )}
        <Field label={t('common.name')} error={errors.name} required>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              touch('name');
            }}
            autoComplete="off"
            autoFocus={editing}
          />
        </Field>
        <Field label={t('team.users.role')} hint={t(`team.users.roleDesc.${role}`)} error={errors.role}>
          <Select
            value={role}
            onChange={(e) => {
              setRole(e.target.value as Role);
              touch('role');
            }}
          >
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {te(`role.${r}`, r)}
              </option>
            ))}
          </Select>
        </Field>
        <Field
          label={editing ? t('team.users.newPassword') : t('team.users.password')}
          hint={editing ? t('team.users.newPasswordHint') : t('team.users.passwordHint')}
          error={errors.password}
          optional
        >
          <Input
            type="password"
            value={password}
            onChange={(e) => {
              setPassword(e.target.value);
              setLocalErrors({});
              touch('password');
            }}
            autoComplete="new-password"
          />
        </Field>
        <Field label={t('team.users.stripeAccount')} hint={t('team.users.stripeHint')} error={errors.stripe_account_id} optional>
          <Input
            value={stripe}
            onChange={(e) => {
              setStripe(e.target.value);
              touch('stripe_account_id');
              touch('password');
            }}
            placeholder="acct_…"
            autoComplete="off"
            className="font-mono"
          />
        </Field>
        {editing && <SwitchField label={t('team.users.disabledLabel')} description={t('team.users.disabledDesc')} checked={disabled} onCheckedChange={setDisabled} />}
      </form>
    </Modal>
  );
}
