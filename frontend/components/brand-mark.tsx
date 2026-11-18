import type { SVGProps } from 'react';

/** The Keypost key. Drawn on a 24px grid like the lucide icons; takes its colour from `currentColor`. */
export function BrandMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      <rect x="3" y="8" width="8" height="8" rx="2.5" />
      <circle cx="7" cy="12" r="1" fill="currentColor" stroke="none" />
      <path d="M11 12h10M16.5 12v3.2M20 12v2.2" />
    </svg>
  );
}
