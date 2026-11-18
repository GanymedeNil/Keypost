import { X } from 'lucide-react';

/** Removable pill shown when a list is narrowed by an id param (e.g. `?order_id=12`). */
export function FilterChip({ label, removeLabel, onRemove }: { label: string; removeLabel: string; onRemove: () => void }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-accent py-0.5 pl-2.5 pr-1 text-xs font-medium text-primary">
      {label}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`${removeLabel}: ${label}`}
        className="grid size-4 place-items-center rounded-full hover:bg-primary/20"
      >
        <X className="size-3" />
      </button>
    </span>
  );
}
