'use client';

import { useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { Button, type ButtonSize, type ButtonVariant } from './button';
import { useToast } from './toast';

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand('copy');
      ta.remove();
      return ok;
    } catch {
      return false;
    }
  }
}

export function CopyButton({
  value,
  label,
  variant = 'ghost',
  size = 'icon-sm',
  className,
  showText,
}: {
  value: string;
  /** Accessible name, e.g. "license key". */
  label?: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  /** Render "Copy"/"Copied" text next to the icon. */
  showText?: boolean;
}) {
  const t = useT();
  const toast = useToast();
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  async function onClick() {
    const ok = await copyText(value);
    if (!ok) return toast.error(t('toast.copyFailed'));
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1600);
  }

  return (
    <Button
      variant={variant}
      size={showText && size.startsWith('icon') ? 'sm' : size}
      className={className}
      aria-label={label ? t('common.copyValue', { label }) : t('common.copy')}
      title={copied ? t('common.copied') : t('common.copy')}
      onClick={onClick}
    >
      {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />}
      {showText && (copied ? t('common.copied') : t('common.copy'))}
    </Button>
  );
}

/** Read-only value box with a copy button (URLs, tokens, ids). */
export function CopyField({ value, label, mono = true, className }: { value: string; label?: string; mono?: boolean; className?: string }) {
  return (
    <div className={cn('flex min-w-0 items-center gap-1 rounded-md border border-line-strong bg-surface-2 py-0.5 pl-2.5 pr-0.5', className)}>
      <code className={cn('min-w-0 flex-1 select-all truncate text-xs text-fg', !mono && 'font-sans')} title={value}>
        {value}
      </code>
      <CopyButton value={value} label={label} />
    </div>
  );
}
