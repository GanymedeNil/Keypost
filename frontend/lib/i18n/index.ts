import common from './messages/common';
import shell from './messages/shell';
import auth from './messages/auth';
import dashboard from './messages/dashboard';
import orders from './messages/orders';
import customers from './messages/customers';
import subscriptions from './messages/subscriptions';
import licenses from './messages/licenses';
import products from './messages/products';
import webhooks from './messages/webhooks';
import stripe from './messages/stripe';
import imports from './messages/imports';
import settings from './messages/settings';
import team from './messages/team';
import mfa from './messages/mfa';
import storefront from './messages/storefront';

export type Lang = 'en' | 'zh';
export const LANGS: readonly Lang[] = ['en', 'zh'];
export const LANG_COOKIE = 'df-lang';
export const LOCALES: Record<Lang, string> = { en: 'en', zh: 'zh-CN' };
export const LANG_LABELS: Record<Lang, string> = { en: 'English', zh: '简体中文' };

const groups = [
  common,
  shell,
  auth,
  dashboard,
  orders,
  customers,
  subscriptions,
  licenses,
  products,
  webhooks,
  stripe,
  imports,
  settings,
  team,
  mfa,
  storefront,
] as const;

type UnionToIntersection<U> = (U extends unknown ? (k: U) => void : never) extends (k: infer I) => void ? I : never;
export type Messages = UnionToIntersection<(typeof groups)[number]['en']>;
export type MessageKey = keyof Messages;

function merge(lang: Lang): Record<string, string> {
  return Object.assign({}, ...groups.map((g) => g[lang])) as Record<string, string>;
}

export const dictionaries: Record<Lang, Record<string, string>> = {
  en: merge('en'),
  zh: merge('zh'),
};

export function parseLang(value: string | null | undefined): Lang | null {
  if (!value) return null;
  const v = value.toLowerCase();
  if (v.startsWith('zh')) return 'zh';
  if (v.startsWith('en')) return 'en';
  return null;
}

/** Pick a language from an Accept-Language header (zh-* → zh, anything else → en). */
export function langFromAcceptLanguage(header: string | null | undefined): Lang {
  if (!header) return 'en';
  for (const part of header.split(',')) {
    const tag = part.split(';')[0]?.trim();
    const l = parseLang(tag);
    if (l) return l;
  }
  return 'en';
}
