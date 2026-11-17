import { cn } from '@/lib/utils';

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn('animate-shimmer rounded-md bg-surface-3', className)} />;
}

/** Generic content placeholder: a title line plus a few text lines. */
export function SkeletonText({ lines = 3, className }: { lines?: number; className?: string }) {
  return (
    <div className={cn('space-y-2', className)} role="status" aria-busy="true">
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} className={cn('h-3.5', i === lines - 1 ? 'w-2/3' : 'w-full')} />
      ))}
    </div>
  );
}

/** Placeholder for a detail page: header + two cards. */
export function PageSkeleton() {
  return (
    <div className="space-y-6" role="status" aria-busy="true">
      <div className="space-y-2">
        <Skeleton className="h-6 w-56" />
        <Skeleton className="h-4 w-80" />
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Skeleton className="h-48 w-full rounded-xl" />
          <Skeleton className="h-40 w-full rounded-xl" />
        </div>
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    </div>
  );
}
