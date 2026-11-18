import { useId, type SVGProps } from 'react';
import { BODY_PATH, LID_PATH, LOGO_VIEWBOX, MARK_VIEWBOX, SPEED_LINES, WORD_PATH } from './brand-paths';

function Mark({ id }: { id: string }) {
  return (
    <>
      <mask id={id} maskUnits="userSpaceOnUse" x="0" y="0" width="1000" height="320">
        <rect width="1000" height="320" fill="#fff" />
        <circle cx="215" cy="192" r="32" fill="#000" />
        <circle cx="215" cy="192" r="14" fill="#fff" />
        <rect x="240" y="182" width="88" height="20" rx="5" fill="#000" />
        <rect x="281" y="198" width="15" height="20" rx="2" fill="#000" />
        <rect x="304" y="198" width="15" height="20" rx="2" fill="#000" />
      </mask>
      <path d={LID_PATH} />
      <g mask={`url(#${id})`}>
        {SPEED_LINES.map(([x, y, w, h]) => (
          <rect key={y} x={x} y={y} width={w} height={h} rx={12} />
        ))}
        <path d={BODY_PATH} />
      </g>
    </>
  );
}

/** The Keypost mark: an envelope with a key and speed lines. Takes its colour from `currentColor`; the key is a real cut-out. */
export function BrandMark(props: SVGProps<SVGSVGElement>) {
  const id = useId();
  return (
    <svg viewBox={MARK_VIEWBOX} fill="currentColor" aria-hidden="true" {...props}>
      <Mark id={id} />
    </svg>
  );
}

/** Mark and wordmark together. */
export function BrandLogo({ label = 'Keypost', ...props }: SVGProps<SVGSVGElement> & { label?: string }) {
  const id = useId();
  return (
    <svg viewBox={LOGO_VIEWBOX} fill="currentColor" role="img" aria-label={label} {...props}>
      <Mark id={id} />
      <path d={WORD_PATH} />
    </svg>
  );
}
