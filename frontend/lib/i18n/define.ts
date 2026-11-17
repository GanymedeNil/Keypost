/**
 * Each feature area ships its English and Simplified Chinese strings side by side.
 * The generic forces `zh` to contain exactly the keys of `en` (compile-time check).
 */
export function defineMessages<const E extends Record<string, string>>(m: {
  en: E;
  zh: Record<keyof NoInfer<E>, string>;
}): { en: E; zh: Record<keyof E, string> } {
  return m;
}
