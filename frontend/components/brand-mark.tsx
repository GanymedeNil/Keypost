import { useId, type SVGProps } from 'react';

/**
 * The Keypost mark: an envelope with a key through its edge. Takes its colour from `currentColor`
 * (the gap around the key is cut out, so it works on any background).
 */
export function BrandMark(props: SVGProps<SVGSVGElement>) {
  const id = useId();
  return (
    <svg viewBox="90 78 200 154" aria-hidden="true" {...props}>
      <mask id={id} maskUnits="userSpaceOnUse" x="90" y="78" width="200" height="154">
        <rect x="90" y="78" width="200" height="154" fill="#fff" />
        <path d="M96 118 L180 186 L264 118 M96 226 L150 172 M264 226 L210 172" fill="none" stroke="#000" strokeWidth="8" />
        <path d="M243 141.6 A31 31 0 1 1 267 141.6 L267 178 L264 180 L264 188 L267 190 L267 196 L259 198 L259 206 L267 208 L267 217 Q267 227 257 227 L250 227 Q243 227 243 217 Z" fill="#000" stroke="#000" strokeWidth="20" strokeLinejoin="round" />
      </mask>
      <rect x="94" y="118" width="172" height="109" fill="currentColor" mask={`url(#${id})`} />
      <path d="M243 141.6 A31 31 0 1 1 267 141.6 L267 178 L264 180 L264 188 L267 190 L267 196 L259 198 L259 206 L267 208 L267 217 Q267 227 257 227 L250 227 Q243 227 243 217 Z M264 105 A9 9 0 1 0 246 105 A9 9 0 1 0 264 105 Z" fill="currentColor" fillRule="evenodd" />
    </svg>
  );
}
