'use client';

import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { PageMeta } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { formatNumber } from '@/lib/format';
import { cn } from '@/lib/utils';
import { Button } from './button';

function pages(page: number, last: number): (number | 'gap')[] {
  const set = new Set([1, last, page - 1, page, page + 1]);
  const nums = [...set].filter((n) => n >= 1 && n <= last).sort((a, b) => a - b);
  const out: (number | 'gap')[] = [];
  nums.forEach((n, i) => {
    if (i > 0 && n - nums[i - 1] > 1) out.push('gap');
    out.push(n);
  });
  return out;
}

export function Pagination({
  meta,
  onPageChange,
  className,
}: {
  meta: PageMeta;
  onPageChange: (page: number) => void;
  className?: string;
}) {
  const { t, locale } = useI18n();
  const from = meta.total === 0 ? 0 : (meta.page - 1) * meta.per_page + 1;
  const to = Math.min(meta.total, meta.page * meta.per_page);
  return (
    <nav aria-label={t('common.pagination')} className={cn('flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-2.5', className)}>
      <p className="text-xs text-subtle" aria-live="polite">
        {t('common.showing', { from: formatNumber(from, locale), to: formatNumber(to, locale), total: formatNumber(meta.total, locale) })}
      </p>
      {meta.last_page > 1 && (
        <div className="flex items-center gap-1">
          <Button size="icon-sm" variant="ghost" aria-label={t('common.previous')} disabled={meta.page <= 1} onClick={() => onPageChange(meta.page - 1)}>
            <ChevronLeft className="size-4" />
          </Button>
          <div className="hidden items-center gap-1 sm:flex">
            {pages(meta.page, meta.last_page).map((p, i) =>
              p === 'gap' ? (
                <span key={`g${i}`} className="px-1 text-xs text-subtle">
                  …
                </span>
              ) : (
                <Button
                  key={p}
                  size="icon-sm"
                  variant={p === meta.page ? 'secondary' : 'ghost'}
                  aria-current={p === meta.page ? 'page' : undefined}
                  className={cn('text-xs tabular-nums', p === meta.page && 'border-accent text-accent-text')}
                  onClick={() => onPageChange(p)}
                >
                  {p}
                </Button>
              ),
            )}
          </div>
          <span className="px-2 text-xs text-muted sm:hidden">{t('common.pageOf', { page: meta.page, last: meta.last_page })}</span>
          <Button size="icon-sm" variant="ghost" aria-label={t('common.next')} disabled={meta.page >= meta.last_page} onClick={() => onPageChange(meta.page + 1)}>
            <ChevronRight className="size-4" />
          </Button>
        </div>
      )}
    </nav>
  );
}
