'use client';

import { useState } from 'react';
import { Search } from 'lucide-react';
import { cn, humanize } from '@/lib/utils';
import { useI18n } from '@/lib/i18n/provider';
import { Button, Checkbox, Skeleton } from '@/components/ui';
import { groupEvents } from './use-webhook-events';

/** Grouped multi-select for webhook events with per-group and global select all / clear. */
export function EventPicker({
  events,
  value,
  onChange,
  loading,
  error,
  invalid,
}: {
  events: string[] | undefined;
  value: string[];
  onChange: (next: string[]) => void;
  loading?: boolean;
  error?: boolean;
  invalid?: boolean;
}) {
  const { t, te } = useI18n();
  const [filter, setFilter] = useState('');

  if (loading && !events) {
    return (
      <div className="space-y-2 rounded-lg border border-line p-3" role="status" aria-busy="true">
        <Skeleton className="h-7 w-full" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/3" />
      </div>
    );
  }
  if (error || !events) {
    return <p className="rounded-lg border border-danger/30 bg-danger-soft p-3 text-xs text-danger-text">{t('webhooks.events.loadFailed')}</p>;
  }

  const selected = new Set(value);
  const needle = filter.trim().toLowerCase();
  const visible = needle ? events.filter((e) => e.toLowerCase().includes(needle)) : events;
  const groups = groupEvents(visible);

  const setMany = (list: string[], on: boolean) => {
    const next = new Set(value);
    for (const e of list) {
      if (on) next.add(e);
      else next.delete(e);
    }
    // keep API order for a stable payload
    onChange(events.filter((e) => next.has(e)));
  };

  return (
    <div className={cn('rounded-lg border bg-surface', invalid ? 'border-danger' : 'border-line-strong')}>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-2">
        <div className="relative min-w-40 flex-1">
          <Search aria-hidden className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t('webhooks.events.filter')}
            aria-label={t('webhooks.events.filter')}
            className="h-7 w-full rounded-md border border-line bg-surface pl-7 pr-2 text-xs placeholder:text-subtle focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 [&::-webkit-search-cancel-button]:hidden"
          />
        </div>
        <span className="text-xs tabular-nums text-subtle">{t('webhooks.events.selectedOf', { count: value.length, total: events.length })}</span>
        <Button size="sm" variant="ghost" onClick={() => setMany(visible, true)}>
          {t('webhooks.events.selectAll')}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => onChange([])} disabled={value.length === 0}>
          {t('webhooks.events.clear')}
        </Button>
      </div>
      <div className="max-h-64 overflow-y-auto p-2">
        {groups.length === 0 ? (
          <p className="px-1 py-3 text-center text-xs text-subtle">{t('webhooks.events.none')}</p>
        ) : (
          groups.map(({ group, events: list }) => {
            const label = te(`webhooks.events.group.${group}`, humanize(group));
            const all = list.every((e) => selected.has(e));
            return (
              <fieldset key={group} className="mb-2 last:mb-0">
                <legend className="sr-only">{label}</legend>
                <div className="mb-1 flex items-center justify-between rounded-md bg-surface-2 px-2 py-1">
                  <Checkbox
                    checked={all}
                    onCheckedChange={(on) => setMany(list, on)}
                    aria-label={t('webhooks.events.groupToggle', { group: label })}
                    label={<span className="text-xs font-semibold">{label}</span>}
                  />
                  <span className="text-[11px] tabular-nums text-subtle">
                    {list.filter((e) => selected.has(e)).length}/{list.length}
                  </span>
                </div>
                <div className="grid gap-x-3 gap-y-1 px-2 sm:grid-cols-2">
                  {list.map((e) => (
                    <Checkbox
                      key={e}
                      checked={selected.has(e)}
                      onCheckedChange={(on) => setMany([e], on)}
                      label={<code className="break-all text-xs">{e}</code>}
                    />
                  ))}
                </div>
              </fieldset>
            );
          })
        )}
      </div>
    </div>
  );
}
