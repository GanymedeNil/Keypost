import type { SVGProps } from 'react';
import { FLIP, LOGO_VIEWBOX, MARK_PATH, MARK_VIEWBOX, WORD_FLIP, WORD_PATH } from './brand-paths';

/** The Keypost mark (envelope, key and speed lines). Takes its colour from `currentColor`; the key is cut out, so it works on any background. */
export function BrandMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox={MARK_VIEWBOX} aria-hidden="true" {...props}>
      <g transform={FLIP} fill="currentColor">
        <path d={MARK_PATH} />
      </g>
    </svg>
  );
}

/** Mark and wordmark together. */
export function BrandLogo({ label = 'Keypost', ...props }: SVGProps<SVGSVGElement> & { label?: string }) {
  return (
    <svg viewBox={LOGO_VIEWBOX} role="img" aria-label={label} {...props}>
      <g fill="currentColor">
        <g transform={FLIP}>
          <path d={MARK_PATH} />
        </g>
        <g transform={WORD_FLIP}>
          <path d={WORD_PATH} />
        </g>
      </g>
    </svg>
  );
}
