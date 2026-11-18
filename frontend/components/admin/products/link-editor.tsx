'use client';

import { Plus, Trash2 } from 'lucide-react';
import { useT } from '@/lib/i18n/provider';
import type { VariantLink } from '@/lib/types';
import { Button, Input } from '@/components/kit';

/** Repeatable title + url rows (links delivered to the customer with the order). */
export function LinkEditor({ links, onChange }: { links: VariantLink[]; onChange: (links: VariantLink[]) => void }) {
  const t = useT();
  const update = (i: number, patch: Partial<VariantLink>) => onChange(links.map((l, idx) => (idx === i ? { ...l, ...patch } : l)));
  return (
    <div className="space-y-2">
      {links.length === 0 && <p className="text-xs text-muted-foreground">{t('products.links.empty')}</p>}
      {links.map((l, i) => (
        <div key={i} className="grid grid-cols-[1fr_auto] gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]">
          <Input
            aria-label={t('common.title')}
            placeholder={t('products.links.titlePlaceholder')}
            value={l.title}
            onChange={(e) => update(i, { title: e.target.value })}
            className="col-span-1"
          />
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('common.remove')}
            className="row-start-1 col-start-2 sm:col-start-3 hover:text-danger-text"
            onClick={() => onChange(links.filter((_, idx) => idx !== i))}
          >
            <Trash2 className="size-4" />
          </Button>
          <Input
            aria-label={t('common.url')}
            type="url"
            inputMode="url"
            placeholder="https://"
            value={l.url}
            onChange={(e) => update(i, { url: e.target.value })}
            className="col-span-2 sm:col-span-1 sm:row-start-1 sm:col-start-2"
          />
        </div>
      ))}
      <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => onChange([...links, { title: '', url: '' }])}>
        {t('products.links.add')}
      </Button>
    </div>
  );
}
