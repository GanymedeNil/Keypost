'use client';

import type { ReactNode } from 'react';
import { AlertTriangle, Inbox } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/lib/i18n/provider';
import { errorText } from '@/lib/error-text';
import { ApiError } from '@/lib/api';
import { Button } from './button';

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('flex flex-col items-center justify-center gap-2 px-6 py-14 text-center', className)}>
      <div className="grid size-10 place-items-center rounded-full bg-accent text-muted-foreground">{icon ?? <Inbox className="size-5" />}</div>
      <p className="text-sm font-medium text-foreground">{title}</p>
      {description && <p className="max-w-sm text-xs text-muted-foreground">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function ErrorState({
  error,
  onRetry,
  title,
  className,
}: {
  error?: unknown;
  onRetry?: () => void;
  title?: ReactNode;
  className?: string;
}) {
  const t = useT();
  const notFound = error instanceof ApiError && error.status === 404;
  return (
    <div role="alert" className={cn('flex flex-col items-center justify-center gap-2 px-6 py-14 text-center', className)}>
      <div className="grid size-10 place-items-center rounded-full bg-danger-soft text-danger-text">
        <AlertTriangle className="size-5" />
      </div>
      <p className="text-sm font-medium text-foreground">{title ?? (notFound ? t('errors.notFound') : t('errors.loadFailed'))}</p>
      {error !== undefined && !notFound && <p className="max-w-md text-xs text-muted-foreground">{errorText(error, t)}</p>}
      {onRetry && !notFound && (
        <Button size="sm" className="mt-2" onClick={onRetry}>
          {t('common.retry')}
        </Button>
      )}
    </div>
  );
}

/** Inline notice box (info/warn/danger) used for hints and warnings inside pages. */
export function Callout({
  tone = 'info',
  title,
  children,
  action,
  className,
}: {
  tone?: 'info' | 'warning' | 'danger' | 'success';
  title?: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  const tones = {
    info: 'border-info/30 bg-info-soft text-info-text',
    warning: 'border-warning/30 bg-warning-soft text-warning-text',
    danger: 'border-danger/30 bg-danger-soft text-danger-text',
    success: 'border-success/30 bg-success-soft text-success-text',
  } as const;
  return (
    <div className={cn('flex flex-wrap items-start justify-between gap-3 rounded-lg border px-3.5 py-3 text-sm', tones[tone], className)}>
      <div className="min-w-0 flex-1">
        {title && <p className="font-medium">{title}</p>}
        {children && <div className={cn('text-[13px]', title && 'mt-0.5 opacity-90')}>{children}</div>}
      </div>
      {action}
    </div>
  );
}
