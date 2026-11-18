'use client';

import { useMemo, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { ExternalLink, Plus, Trash2 } from 'lucide-react';
import { adminApi, ApiError } from '@/lib/api';
import { useI18n } from '@/lib/i18n/provider';
import { useAction } from '@/lib/use-action';
import { errorText } from '@/lib/error-text';
import { useApi } from '@/lib/use-api';
import type { Product } from '@/lib/types';
import {
  Button,
  ButtonAnchor,
  Card,
  CardBody,
  CardHeader,
  CopyButton,
  DateText,
  DI,
  DL,
  EmptyState,
  ErrorState,
  PageHeader,
  PageSkeleton,
  StatusBadge,
  TestModeBadge,
  useConfirmDelete,
  useToast,
} from '@/components/kit';
import { useAdmin, useCan } from '@/components/admin/admin-context';
import { NewVariantDialog } from '@/components/admin/products/new-variant-dialog';
import { ProductForm } from '@/components/admin/products/product-form';
import { Thumb } from '@/components/admin/products/thumb';
import { VariantCard } from '@/components/admin/products/variant-card';

export default function ProductDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t } = useI18n();
  const router = useRouter();
  const toast = useToast();
  const can = useCan();
  const { stores } = useAdmin();
  const confirmDelete = useConfirmDelete();
  const { data: product, error, isLoading, mutate } = useApi<Product>(`/products/${id}`);
  const move = useAction();
  const del = useAction();
  const [adding, setAdding] = useState(false);
  const [justCreated, setJustCreated] = useState<number | null>(null);

  const variants = useMemo(() => [...(product?.variants ?? [])].sort((a, b) => a.sort - b.sort || a.id - b.id), [product]);
  const refresh = () => mutate();

  if (error && !product) {
    return (
      <>
        <PageHeader title={t('products.title')} back={{ href: '/admin/products', label: t('products.back') }} />
        <Card>
          <ErrorState error={error} onRetry={() => void mutate()} />
        </Card>
      </>
    );
  }
  if (!product) return isLoading ? <PageSkeleton /> : null;

  const currency = stores.find((s) => s.id === product.store_id)?.currency ?? 'USD';

  async function reorder(index: number, direction: -1 | 1) {
    const order = [...variants];
    const target = index + direction;
    [order[index], order[target]] = [order[target], order[index]];
    const updates = order.map((v, i) => ({ v, sort: i + 1 })).filter(({ v, sort }) => v.sort !== sort);
    const res = await move.run(() => Promise.all(updates.map(({ v, sort }) => adminApi.patch(`/variants/${v.id}`, { sort }))));
    if (res.ok) await refresh();
  }

  async function remove() {
    if (!product) return;
    if (!(await confirmDelete(product.name, t('products.deleteBody')))) return;
    const res = await del.run(() => adminApi.delete(`/products/${product.id}`), { success: t('toast.deleted'), silent: true });
    if (res.ok) router.push('/admin/products');
    else toast.error(res.error instanceof ApiError && res.error.code === 'conflict' ? t('products.deleteConflict') : errorText(res.error, t));
  }

  return (
    <>
      <PageHeader
        title={product.name}
        back={{ href: '/admin/products', label: t('products.back') }}
        badges={
          <>
            <StatusBadge status={product.status} />
            {product.test_mode && <TestModeBadge />}
          </>
        }
        description={product.slug}
        actions={
          <>
            {product.buy_now_url && (
              <>
                <ButtonAnchor href={product.buy_now_url} target="_blank" rel="noreferrer" icon={<ExternalLink className="size-3.5" />}>
                  {t('products.openBuyLink')}
                </ButtonAnchor>
                <CopyButton value={product.buy_now_url} label={t('products.buyLink')} variant="secondary" size="icon" />
              </>
            )}
            {can.write && (
              <Button variant="danger-outline" icon={<Trash2 className="size-3.5" />} loading={del.pending} onClick={remove}>
                {t('common.delete')}
              </Button>
            )}
          </>
        }
      />

      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className="space-y-5">
          <ProductForm key={product.updated_at} product={product} onSaved={refresh} />
          <Card>
            <CardHeader title={t('products.info.title')} />
            <CardBody>
              <div className="mb-4 flex items-center gap-3">
                <Thumb url={product.large_thumb_url || product.thumb_url} label={product.name} className="size-14" />
                <p className="text-xs text-muted-foreground">{t('products.info.thumbHint')}</p>
              </div>
              <DL cols={2}>
                <DI label={t('common.id')}>{product.id}</DI>
                <DI label={t('products.col.variants')}>{product.variants_count}</DI>
                <DI label={t('common.created')}>
                  <DateText value={product.created_at} />
                </DI>
                <DI label={t('common.updated')}>
                  <DateText value={product.updated_at} />
                </DI>
              </DL>
            </CardBody>
          </Card>
        </div>

        <section aria-labelledby="variants-heading" className="min-w-0 space-y-3">
          <div className="flex items-center justify-between gap-2">
            <div>
              <h2 id="variants-heading" className="text-sm font-semibold">
                {t('products.variants.title')}
              </h2>
              <p className="text-xs text-muted-foreground">{t('products.variants.hint')}</p>
            </div>
            {can.write && (
              <Button variant="primary" size="sm" icon={<Plus className="size-3.5" />} onClick={() => setAdding(true)}>
                {t('products.variant.add')}
              </Button>
            )}
          </div>
          {variants.length === 0 ? (
            <Card>
              <EmptyState
                title={t('products.variants.empty')}
                description={t('products.variants.emptyHint')}
                action={
                  can.write ? (
                    <Button variant="primary" size="sm" onClick={() => setAdding(true)}>
                      {t('products.variant.add')}
                    </Button>
                  ) : undefined
                }
              />
            </Card>
          ) : (
            <div className="space-y-3">
              {variants.map((v, i) => (
                <VariantCard
                  key={v.id}
                  variant={v}
                  currency={currency}
                  defaultOpen={v.id === justCreated || variants.length === 1}
                  canMoveUp={i > 0}
                  canMoveDown={i < variants.length - 1}
                  onMove={(dir) => void reorder(i, dir)}
                  onChanged={refresh}
                />
              ))}
            </div>
          )}
        </section>
      </div>

      <NewVariantDialog
        open={adding}
        onOpenChange={setAdding}
        productId={product.id}
        currency={currency}
        onCreated={async (v) => {
          setJustCreated(v.id);
          await refresh();
        }}
      />
    </>
  );
}
