'use client';

import { Badge, Button, Field, Input, useConfirm } from '@/components/ui';
import { useI18n } from '@/lib/i18n/provider';
import type { SecretSetting } from '@/lib/types';

/** Pending edit of a write-only setting: `value` = replace, `clear` = remove the stored value. */
export interface SecretDraft {
  value: string;
  clear: boolean;
}

export const emptySecret: SecretDraft = { value: '', clear: false };

/** Value for the PUT body: string = set, "" = clear, undefined = keep. */
export function secretUpdate(d: SecretDraft): string | undefined {
  if (d.value) return d.value;
  return d.clear ? '' : undefined;
}

export function SecretField({
  label,
  current,
  draft,
  onChange,
  disabled,
  hint,
  error,
}: {
  label: string;
  current: SecretSetting;
  draft: SecretDraft;
  onChange: (next: SecretDraft) => void;
  disabled?: boolean;
  hint?: string;
  error?: string;
}) {
  const { t, te } = useI18n();
  const confirm = useConfirm();

  async function stageClear() {
    const ok = await confirm({
      title: t('settings.secret.clearTitle', { name: label }),
      description: t('settings.secret.clearBody'),
      confirmLabel: t('settings.secret.clear'),
    });
    if (ok) onChange({ value: '', clear: true });
  }

  return (
    <Field label={label} hint={hint ?? t('settings.secret.writeOnly')} error={error}>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        {current.set ? (
          <>
            <Badge tone="success">{t('settings.secret.set')}</Badge>
            {current.source && <Badge>{te(`settings.secret.source.${current.source}`, current.source)}</Badge>}
            {current.hint && <code className="rounded bg-surface-3 px-1.5 py-0.5 text-xs">{current.hint}</code>}
          </>
        ) : (
          <Badge>{t('settings.secret.notSet')}</Badge>
        )}
        {draft.clear && (
          <span className="flex items-center gap-1.5 text-danger-text">
            {t('settings.secret.willClear')}
            <button type="button" className="underline underline-offset-2" onClick={() => onChange(emptySecret)}>
              {t('settings.secret.undo')}
            </button>
          </span>
        )}
      </div>
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <Input
            type="password"
            autoComplete="new-password"
            spellCheck={false}
            disabled={disabled || draft.clear}
            value={draft.value}
            placeholder={current.set ? t('settings.secret.placeholderReplace') : t('settings.secret.placeholderNew')}
            onChange={(e) => onChange({ value: e.target.value, clear: false })}
          />
        </div>
        {current.set && !draft.clear && !disabled && (
          <Button variant="danger-outline" onClick={stageClear}>
            {t('settings.secret.clear')}
          </Button>
        )}
      </div>
      {current.set && current.source === 'env' && <p className="text-xs text-warning-text">{t('settings.secret.envOverride')}</p>}
    </Field>
  );
}
