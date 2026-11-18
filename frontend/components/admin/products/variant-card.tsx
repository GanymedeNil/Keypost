'use client';

import { useId, useState } from 'react';
import { ArrowDown, ArrowUp, ChevronRight, KeyRound, Loader2, Paperclip } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { useApi } from '@/lib/use-api';
import { cn } from '@/lib/utils';
import type { Variant } from '@/lib/types';
import { Badge, Button, StatusBadge, Tabs, type TabItem } from '@/components/kit';
import { useCan } from '@/components/admin/admin-context';
import { DetailsTab } from './details-tab';
import { FilesTab } from './files-tab';
import { LicenseTab } from './license-tab';
import { priceLabel } from './price-utils';
import { PricingTab } from './pricing-tab';
import { StripeTab } from './stripe-tab';
import { useUploadQueue } from './upload-queue';

type TabName = 'details' | 'pricing' | 'license' | 'files' | 'stripe';

export function VariantCard({
  variant,
  currency,
  canMoveUp,
  canMoveDown,
  onMove,
  onChanged,
  defaultOpen = false,
}: {
  variant: Variant;
  currency: string;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onMove: (direction: -1 | 1) => void;
  /** Refresh the product (and its variants). */
  onChanged: () => Promise<unknown>;
  defaultOpen?: boolean;
}) {
  const i18n = useI18n();
  const { t } = i18n;
  const can = useCan();
  const panelId = useId();
  const [open, setOpen] = useState(defaultOpen);
  const [tab, setTab] = useState<TabName>('details');
  const { queue, items } = useUploadQueue(variant.id);
  const detail = useApi<Variant>(open && tab === 'files' ? `/variants/${variant.id}` : null);

  const changed = async () => {
    await Promise.all([onChanged(), detail.mutate()]);
  };
  const uploading = items.filter((i) => i.state === 'uploading' || i.state === 'queued').length;

  const tabs: TabItem[] = [
    { value: 'details', label: t('products.tab.details') },
    { value: 'pricing', label: t('products.tab.pricing'), count: variant.prices.length },
    { value: 'license', label: t('products.tab.license') },
    { value: 'files', label: t('products.tab.files'), count: variant.files_count },
    { value: 'stripe', label: t('products.tab.stripe') },
  ];

  const shown = variant.prices.slice(0, 3);
  const extra = variant.prices.length - shown.length;

  return (
    <div className="rounded-xl border border-border bg-card shadow-card">
      <div className="flex items-start gap-1 p-2 sm:p-3">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen((o) => !o)}
          className="flex min-w-0 flex-1 items-start gap-2 rounded-lg p-1.5 text-left hover:bg-muted/70"
        >
          <ChevronRight className={cn('mt-1 size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-90')} />
          <span className="min-w-0 flex-1 space-y-1.5">
            <span className="flex flex-wrap items-center gap-2">
              <span className="truncate text-sm font-semibold">{variant.name}</span>
              <StatusBadge status={variant.status} />
              {variant.has_license_keys && (
                <Badge tone="accent">
                  <KeyRound className="size-3" />
                  {t('products.variant.licenseBadge')}
                </Badge>
              )}
              {variant.stripe_product_id ? (
                <Badge tone="success">{t('products.stripe.mapped')}</Badge>
              ) : (
                <Badge tone="warning">{t('products.stripe.notMapped')}</Badge>
              )}
            </span>
            <span className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
              {variant.prices.length === 0 ? (
                <span className="text-muted-foreground">{t('products.pricing.empty')}</span>
              ) : (
                <>
                  {shown.map((p) => (
                    <span key={p.id} className="rounded-md bg-accent px-1.5 py-0.5 tabular-nums">
                      {priceLabel(p, i18n)}
                    </span>
                  ))}
                  {extra > 0 && <span className="text-muted-foreground">+{extra}</span>}
                </>
              )}
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Paperclip className="size-3" />
                {variant.files_count}
              </span>
              {uploading > 0 && (
                <span className="inline-flex items-center gap-1 text-primary">
                  <Loader2 className="size-3 animate-spin-slow" />
                  {t('products.files.uploadingCount', { count: uploading })}
                </span>
              )}
            </span>
          </span>
        </button>
        {can.write && (
          <div className="flex shrink-0 items-center">
            <Button size="icon-sm" variant="ghost" aria-label={t('products.variant.moveUp')} disabled={!canMoveUp} onClick={() => onMove(-1)}>
              <ArrowUp className="size-3.5" />
            </Button>
            <Button size="icon-sm" variant="ghost" aria-label={t('products.variant.moveDown')} disabled={!canMoveDown} onClick={() => onMove(1)}>
              <ArrowDown className="size-3.5" />
            </Button>
          </div>
        )}
      </div>

      {open && (
        <div id={panelId} className="border-t border-border">
          <Tabs items={tabs} value={tab} onValueChange={(v) => setTab(v as TabName)} ariaLabel={variant.name} className="px-2 sm:px-3" />
          <div role="tabpanel" className="p-3 sm:p-4">
            {tab === 'details' && <DetailsTab key={variant.updated_at} variant={variant} onChanged={onChanged} onDeleted={onChanged} />}
            {tab === 'pricing' && <PricingTab variant={variant} currency={currency} onChanged={onChanged} />}
            {tab === 'license' && <LicenseTab key={variant.updated_at} variant={variant} onChanged={onChanged} />}
            {tab === 'files' && (
              <FilesTab
                files={detail.data?.files}
                loading={detail.isLoading}
                error={detail.error}
                onRetry={() => void detail.mutate()}
                queue={queue}
                items={items}
                onChanged={changed}
              />
            )}
            {tab === 'stripe' && <StripeTab key={variant.updated_at} variant={variant} onChanged={onChanged} />}
          </div>
        </div>
      )}
    </div>
  );
}
