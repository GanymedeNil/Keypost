import { Package } from 'lucide-react';
import { cn } from '@/lib/utils';

/** Product thumbnail rendered as a CSS background (works with any remote URL, no <img> lint/layout concerns). */
export function Thumb({ url, label, className }: { url?: string | null; label: string; className?: string }) {
  const safe = url && /^(https?:)?\/\//.test(url) ? url.replace(/["()\\]/g, encodeURIComponent) : null;
  return (
    <span
      role={safe ? 'img' : undefined}
      aria-label={safe ? label : undefined}
      aria-hidden={safe ? undefined : true}
      style={safe ? { backgroundImage: `url("${safe}")` } : undefined}
      className={cn('grid size-9 shrink-0 place-items-center overflow-hidden rounded-lg border border-border bg-accent bg-cover bg-center text-muted-foreground', className)}
    >
      {!safe && <Package className="size-4" />}
    </span>
  );
}
