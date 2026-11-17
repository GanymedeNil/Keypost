'use client';

import type { ReactNode } from 'react';
import { ExternalLink, FileDown, Link2, Receipt } from 'lucide-react';
import type { StorefrontOrder } from '@/lib/types';
import { useI18n } from '@/lib/i18n/provider';
import { formatDate } from '@/lib/format';
import { cn } from '@/lib/utils';
import { ButtonAnchor, Card, CardHeader, CopyButton, Progress, StatusBadge } from '@/components/ui';

type LicenseKey = StorefrontOrder['license_keys'][number];
type DownloadFile = StorefrontOrder['files'][number];

export function Section({ title, description, children, className }: { title: string; description?: string; children: ReactNode; className?: string }) {
  return (
    <Card className={cn('print:break-inside-avoid print:shadow-none', className)}>
      <CardHeader title={title} description={description} />
      {children}
    </Card>
  );
}

export function ProductLabel({ product, variant }: { product: string; variant?: string | null }) {
  return (
    <p className="min-w-0 text-sm">
      <span className="font-medium text-fg">{product}</span>
      {variant && <span className="text-muted"> — {variant}</span>}
    </p>
  );
}

function LicenseKeyRow({ item }: { item: LicenseKey }) {
  const { t, locale } = useI18n();
  const limit = item.activation_limit;
  return (
    <li className="space-y-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <ProductLabel product={item.product_name} variant={item.variant_name} />
        <StatusBadge status={item.status} />
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <code
          className="block min-w-0 flex-1 select-all break-all rounded-lg border border-line-strong bg-surface-2 px-3 py-2.5 text-sm leading-snug text-fg"
          aria-label={t('storefront.order.licenseKeys')}
        >
          {item.key}
        </code>
        <CopyButton value={item.key} label={t('storefront.order.licenseKeys')} variant="secondary" size="lg" showText className="shrink-0 print:hidden" />
      </div>
      <div className="space-y-1.5">
        <p className="text-xs text-muted">
          {limit === null
            ? t('storefront.order.activationsUnlimited', { used: item.instances_count })
            : t('storefront.order.activations', { used: item.instances_count, limit })}
        </p>
        {limit !== null && limit > 0 && (
          <Progress
            value={item.instances_count}
            max={limit}
            tone={item.instances_count >= limit ? 'warning' : 'accent'}
            label={t('storefront.order.activations', { used: item.instances_count, limit })}
          />
        )}
        <p className="text-xs text-subtle">
          {item.expires_at
            ? t('storefront.order.expires', { date: formatDate(item.expires_at, locale, 'date') })
            : t('storefront.order.neverExpires')}
        </p>
      </div>
    </li>
  );
}

export function LicenseKeysSection({ keys }: { keys: LicenseKey[] }) {
  const { t } = useI18n();
  return (
    <Section title={t('storefront.order.licenseKeys')} description={t('storefront.order.licenseKeysHint')}>
      <ul className="divide-y divide-line">
        {keys.map((k, i) => (
          <LicenseKeyRow key={`${k.key}-${i}`} item={k} />
        ))}
      </ul>
    </Section>
  );
}

function FileRow({ file }: { file: DownloadFile }) {
  const { t } = useI18n();
  const filename = file.extension ? `${file.name}.${file.extension.replace(/^\./, '')}` : file.name;
  return (
    <li className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
      <div className="flex min-w-0 items-start gap-3">
        <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg bg-surface-3 text-muted">
          <FileDown className="size-4.5" aria-hidden />
        </span>
        <div className="min-w-0">
          <p className="break-all text-sm font-medium">{filename}</p>
          <p className="text-xs text-subtle">
            {[file.version ? t('storefront.order.version', { version: file.version }) : null, file.size_formatted].filter(Boolean).join(' · ')}
          </p>
          <div className="mt-0.5">
            <ProductLabel product={file.product_name} variant={file.variant_name} />
          </div>
        </div>
      </div>
      <ButtonAnchor href={file.download_url} variant="primary" size="lg" className="h-11 w-full shrink-0 sm:w-auto print:hidden" icon={<FileDown className="size-4" />}>
        {t('storefront.order.download')}
      </ButtonAnchor>
    </li>
  );
}

export function DownloadsSection({ files }: { files: DownloadFile[] }) {
  const { t } = useI18n();
  return (
    <Section title={t('storefront.order.downloads')} description={t('storefront.order.downloadsHint')}>
      <ul className="divide-y divide-line">
        {files.map((f) => (
          <FileRow key={f.id} file={f} />
        ))}
      </ul>
    </Section>
  );
}

export function LinksSection({ links }: { links: StorefrontOrder['links'] }) {
  const { t } = useI18n();
  return (
    <Section title={t('storefront.order.links')}>
      <ul className="divide-y divide-line">
        {links.map((l, i) => (
          <li key={`${l.url}-${i}`}>
            <a
              href={l.url}
              target="_blank"
              rel="noreferrer"
              className="flex min-h-12 items-center justify-between gap-3 px-4 py-3 text-sm hover:bg-surface-2/60 sm:px-5"
            >
              <span className="flex min-w-0 items-center gap-2.5">
                <Link2 className="size-4 shrink-0 text-subtle" aria-hidden />
                <span className="min-w-0">
                  <span className="block truncate font-medium text-accent-text">{l.title || l.url}</span>
                  {l.title && <span className="block truncate text-xs text-subtle">{l.url}</span>}
                </span>
              </span>
              <ExternalLink className="size-4 shrink-0 text-subtle" aria-hidden />
            </a>
          </li>
        ))}
      </ul>
    </Section>
  );
}

export function InvoiceSection({ url }: { url: string }) {
  const { t } = useI18n();
  return (
    <Section title={t('storefront.order.invoice')} description={t('storefront.order.invoiceHint')}>
      <div className="p-4 sm:p-5">
        <ButtonAnchor href={url} target="_blank" rel="noreferrer" size="lg" className="h-11 w-full sm:w-auto print:hidden" icon={<Receipt className="size-4" />}>
          {t('storefront.order.viewInvoice')}
        </ButtonAnchor>
      </div>
    </Section>
  );
}
