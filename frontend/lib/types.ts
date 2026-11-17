/**
 * Types mirroring docs/ADMIN_API.md. Fields that the contract marks optional or
 * that can plausibly be empty are `| null` / optional. Keep in sync with the API.
 */

export type ID = number;

export interface PageMeta {
  page: number;
  per_page: number;
  total: number;
  last_page: number;
}

export interface Paginated<T> {
  data: T[];
  meta: PageMeta;
}

/* ------------------------------------------------------------------ auth */

export type Role = 'owner' | 'admin' | 'viewer';

export interface User {
  id: ID;
  email: string;
  name: string;
  role: Role;
  stripe_account_id: string | null;
  stripe_account_label: string | null;
  disabled: boolean;
  has_password: boolean;
  last_login_at: string | null;
  created_at: string;
}

export interface AuthConfig {
  password_login: boolean;
  stripe_login: boolean;
  setup_required: boolean;
}

/* ---------------------------------------------------------------- stores */

export interface Store {
  id: ID;
  name: string;
  slug: string;
  domain: string | null;
  url: string | null;
  avatar_url: string | null;
  plan: string | null;
  country: string | null;
  currency: string;
  support_email: string | null;
  email_from_name: string | null;
  receipt_subject: string | null;
  receipt_intro: string | null;
  stripe_account_id: string | null;
  total_sales: number;
  total_revenue: number;
  thirty_day_sales: number;
  thirty_day_revenue: number;
  created_at: string;
  updated_at: string;
}

export type StoreInput = Partial<
  Pick<
    Store,
    | 'name'
    | 'slug'
    | 'domain'
    | 'url'
    | 'avatar_url'
    | 'country'
    | 'currency'
    | 'support_email'
    | 'email_from_name'
    | 'receipt_subject'
    | 'receipt_intro'
  >
>;

/* -------------------------------------------------------------- catalog */

export type ProductStatus = 'draft' | 'published';
export type VariantStatus = 'pending' | 'draft' | 'published';
export type PriceCategory = 'one_time' | 'subscription' | 'lead_magnet' | 'pwyw';
export type PriceScheme = 'standard' | 'package' | 'graduated' | 'volume';
export type IntervalUnit = 'day' | 'week' | 'month' | 'year';
export type LicenseLengthUnit = 'days' | 'months' | 'years';

export interface PriceTier {
  last_unit: number | string;
  unit_price: number;
  unit_price_decimal?: string | null;
  fixed_fee?: number | null;
}

export interface Price {
  id: ID;
  variant_id: ID;
  category: PriceCategory;
  scheme: PriceScheme;
  unit_price: number;
  unit_price_decimal: string | null;
  setup_fee_enabled: boolean;
  setup_fee: number | null;
  package_size: number | null;
  tiers: PriceTier[] | null;
  renewal_interval_unit: IntervalUnit | null;
  renewal_interval_quantity: number | null;
  trial_interval_unit: IntervalUnit | null;
  trial_interval_quantity: number | null;
  min_price: number | null;
  suggested_price: number | null;
  tax_code: string | null;
  currency: string;
  stripe_price_id: string | null;
  created_at: string;
  updated_at: string;
}

export type PriceInput = Partial<Omit<Price, 'id' | 'variant_id' | 'created_at' | 'updated_at'>>;

export interface VariantLink {
  title: string;
  url: string;
}

export interface VariantFile {
  id: ID;
  variant_id: ID;
  variant_name: string;
  product_name: string;
  identifier: string;
  name: string;
  extension: string;
  size: number;
  size_formatted: string;
  version: string | null;
  sort: number;
  status: 'draft' | 'published';
  storage_driver: string;
  content_type: string;
  sha256: string | null;
  downloads_count: number;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
}

export interface Variant {
  id: ID;
  product_id: ID;
  product_name: string;
  name: string;
  slug: string;
  description: string | null;
  links: VariantLink[] | null;
  sort: number;
  status: VariantStatus;
  has_license_keys: boolean;
  license_activation_limit: number | null;
  is_license_limit_unlimited: boolean;
  license_length_value: number | null;
  license_length_unit: LicenseLengthUnit | null;
  is_license_length_unlimited: boolean;
  stripe_product_id: string | null;
  buy_now_url: string | null;
  prices: Price[];
  files_count: number;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
  /** detail only */
  files?: VariantFile[];
}

export type VariantInput = Partial<
  Pick<
    Variant,
    | 'product_id'
    | 'name'
    | 'slug'
    | 'description'
    | 'links'
    | 'sort'
    | 'status'
    | 'has_license_keys'
    | 'license_activation_limit'
    | 'is_license_limit_unlimited'
    | 'license_length_value'
    | 'license_length_unit'
    | 'is_license_length_unlimited'
    | 'test_mode'
  >
> & { price?: PriceInput };

export interface Product {
  id: ID;
  store_id: ID;
  name: string;
  slug: string;
  description: string | null;
  status: ProductStatus;
  thumb_url: string | null;
  large_thumb_url: string | null;
  stripe_product_id: string | null;
  test_mode: boolean;
  buy_now_url: string | null;
  variants_count: number;
  from_price: number | null;
  to_price: number | null;
  created_at: string;
  updated_at: string;
  /** detail only */
  variants?: Variant[];
}

export type ProductInput = Partial<
  Pick<
    Product,
    | 'store_id'
    | 'name'
    | 'slug'
    | 'description'
    | 'status'
    | 'thumb_url'
    | 'large_thumb_url'
    | 'stripe_product_id'
    | 'test_mode'
  >
>;

/* ------------------------------------------------------------- customers */

export interface Customer {
  id: ID;
  store_id: ID;
  name: string;
  email: string;
  status: string;
  city: string | null;
  region: string | null;
  country: string | null;
  stripe_customer_id: string | null;
  total_revenue: number;
  mrr: number;
  orders_count: number;
  subscriptions_count: number;
  license_keys_count: number;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
}

export interface CustomerDetail extends Customer {
  orders: Order[];
  subscriptions: Subscription[];
  license_keys: LicenseKey[];
}

/* ---------------------------------------------------------------- orders */

export type OrderStatus = 'pending' | 'failed' | 'paid' | 'refunded' | 'partial_refund' | 'fraudulent';

export interface OrderItem {
  id: ID;
  order_id: ID;
  product_id: ID;
  variant_id: ID;
  price_id: ID;
  product_name: string;
  variant_name: string;
  price: number;
  quantity: number;
}

export interface Order {
  id: ID;
  store_id: ID;
  customer_id: ID;
  identifier: string;
  order_number: number;
  user_name: string;
  user_email: string;
  currency: string;
  currency_rate: number | string | null;
  subtotal: number;
  setup_fee: number;
  discount_total: number;
  tax: number;
  total: number;
  refunded_amount: number;
  subtotal_usd: number;
  discount_total_usd: number;
  tax_usd: number;
  total_usd: number;
  refunded_amount_usd: number;
  tax_name: string | null;
  tax_rate: number | string | null;
  tax_inclusive: boolean;
  status: OrderStatus;
  refunded: boolean;
  refunded_at: string | null;
  affiliate_id: ID | null;
  referral_amount: number | null;
  country: string | null;
  city: string | null;
  postal_code: string | null;
  card_brand: string | null;
  card_last_four: string | null;
  custom_data: unknown;
  stripe_checkout_session: string | null;
  stripe_payment_intent: string | null;
  stripe_invoice_id: string | null;
  stripe_customer_id: string | null;
  source: 'stripe' | 'lemonsqueezy' | 'manual';
  test_mode: boolean;
  receipt_url: string | null;
  first_item: { product_name: string; variant_name: string; price: number; quantity: number } | null;
  created_at: string;
  updated_at: string;
}

export interface OrderDetail extends Order {
  items: OrderItem[];
  license_keys: LicenseKey[];
  subscriptions: Subscription[];
  files: VariantFile[];
  emails: EmailLog[];
  webhook_deliveries: Delivery[];
}

/* --------------------------------------------------------- subscriptions */

export type SubscriptionStatus =
  | 'on_trial'
  | 'active'
  | 'paused'
  | 'past_due'
  | 'unpaid'
  | 'cancelled'
  | 'expired';

export interface Subscription {
  id: ID;
  store_id: ID;
  customer_id: ID;
  order_id: ID;
  order_item_id: ID;
  product_id: ID;
  variant_id: ID;
  price_id: ID;
  product_name: string;
  variant_name: string;
  user_name: string;
  user_email: string;
  status: SubscriptionStatus;
  status_formatted: string;
  card_brand: string | null;
  card_last_four: string | null;
  payment_processor: string | null;
  pause: unknown;
  cancelled: boolean;
  trial_ends_at: string | null;
  billing_anchor: number | null;
  quantity: number;
  renews_at: string | null;
  ends_at: string | null;
  stripe_subscription_id: string | null;
  stripe_customer_id: string | null;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
}

export type InvoiceStatus = 'pending' | 'paid' | 'void' | 'refunded' | 'partial_refund';

export interface SubscriptionInvoice {
  id: ID;
  subscription_id: ID;
  customer_id: ID;
  user_name: string;
  user_email: string;
  billing_reason: string;
  card_brand: string | null;
  card_last_four: string | null;
  currency: string;
  status: InvoiceStatus;
  refunded: boolean;
  refunded_at: string | null;
  subtotal: number;
  discount_total: number;
  tax: number;
  total: number;
  refunded_amount: number;
  invoice_url: string | null;
  stripe_invoice_id: string | null;
  created_at: string;
}

export interface SubscriptionDetail extends Subscription {
  invoices: SubscriptionInvoice[];
  license_keys: LicenseKey[];
}

/* ---------------------------------------------------------- license keys */

export type LicenseStatus = 'inactive' | 'active' | 'expired' | 'disabled';

export interface LicenseKeyInstance {
  id: ID;
  license_key_id: ID;
  identifier: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface LicenseKey {
  id: ID;
  store_id: ID;
  customer_id: ID | null;
  order_id: ID | null;
  order_item_id: ID | null;
  product_id: ID;
  variant_id: ID;
  subscription_id: ID | null;
  product_name: string;
  variant_name: string;
  user_name: string;
  user_email: string;
  key: string;
  key_short: string;
  activation_limit: number | null;
  instances_count: number;
  disabled: boolean;
  status: LicenseStatus;
  status_formatted: string;
  expires_at: string | null;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
}

export interface LicenseKeyDetail extends LicenseKey {
  instances: LicenseKeyInstance[];
}

/* -------------------------------------------------------------- webhooks */

export interface Webhook {
  id: ID;
  store_id: ID;
  url: string;
  events: string[];
  disabled: boolean;
  secret_set: boolean;
  last_sent_at: string | null;
  test_mode: boolean;
  stats: { success_24h: number; failed_24h: number };
  created_at: string;
  updated_at: string;
}

export type DeliveryStatus = 'pending' | 'success' | 'failed';

export interface Delivery {
  id: ID;
  webhook_id: ID;
  webhook_url: string;
  event_name: string;
  resource_type: string | null;
  resource_id: ID | string | null;
  status: DeliveryStatus;
  attempts: number;
  next_attempt_at: string | null;
  last_status_code: number | null;
  last_response: string | null;
  last_error: string | null;
  duration_ms: number | null;
  created_at: string;
  delivered_at: string | null;
}

export interface DeliveryDetail extends Delivery {
  payload: string;
}

/* ---------------------------------------------------------------- stripe */

export type StripeEventStatus = 'received' | 'processed' | 'ignored' | 'failed';

export interface StripeEvent {
  id: ID;
  type: string;
  account: string | null;
  livemode: boolean;
  status: StripeEventStatus;
  attempts: number;
  error: string | null;
  received_at: string;
  processed_at: string | null;
}

export interface StripeEventDetail extends StripeEvent {
  /** Contract does not pin the type: may be a JSON string or an already parsed object. */
  payload: unknown;
}

export interface StripeStatus {
  configured: boolean;
  livemode: boolean;
  account: { id: string; business_name: string | null; country: string | null; email: string | null } | null;
  webhook_secret_set: boolean;
  webhook_url: string;
  required_events: string[];
  error?: string | null;
}

export interface StripeCatalogPrice {
  id: string;
  unit_amount: number | null;
  currency: string;
  recurring: { interval: string; interval_count: number } | null;
  active: boolean;
}

export interface StripeCatalogProduct {
  id: string;
  name: string;
  active: boolean;
  description: string | null;
  metadata: Record<string, string>;
  default_price: string | null;
  prices: StripeCatalogPrice[];
  mapped_variant_id: ID | null;
}

/* ------------------------------------------------------------ misc admin */

export interface ApiKey {
  id: ID;
  name: string;
  prefix: string;
  test_mode: boolean;
  owner_email: string | null;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface ApiKeyCreated extends ApiKey {
  token: string;
}

export interface AuditLog {
  id: ID;
  user_email: string | null;
  action: string;
  resource_type: string | null;
  resource_id: ID | string | null;
  details: unknown;
  ip: string | null;
  created_at: string;
}

export interface EmailLog {
  id: ID;
  order_id: ID | null;
  to: string;
  subject: string;
  template: string | null;
  status: string;
  error: string | null;
  created_at: string;
}

/* ------------------------------------------------------------- dashboard */

export interface DashboardTotals {
  revenue: number;
  orders: number;
  refunded: number;
  customers: number;
  active_subscriptions: number;
  mrr: number;
  license_keys: number;
  active_license_keys: number;
  downloads: number;
}

export interface DashboardEvent {
  kind: 'stripe_event' | 'webhook_delivery' | 'import' | string;
  title: string;
  status: string;
  at: string;
}

export interface DashboardHealth {
  stripe_configured: boolean;
  stripe_webhook_secret_set: boolean;
  smtp_configured: boolean;
  failed_stripe_events: number;
  failed_webhook_deliveries: number;
  pending_webhook_deliveries: number;
  storage_driver: string;
}

export interface Dashboard {
  currency: string;
  totals: DashboardTotals;
  previous: { revenue: number; orders: number };
  series: { date: string; revenue: number; orders: number; refunds: number }[];
  recent_orders: Order[];
  recent_events: DashboardEvent[];
  health: DashboardHealth;
}

/* -------------------------------------------------------------- settings */

export interface SecretSetting {
  set: boolean;
  source?: 'db' | 'env' | string;
  hint?: string;
}

export interface Settings {
  urls: {
    public_url: string;
    stripe_webhook_url: string;
    stripe_oauth_redirect_uri: string;
    api_base_url: string;
  };
  stripe: {
    secret_key: SecretSetting;
    webhook_secret: SecretSetting;
    api_version: string;
    managed_payments: boolean;
    oauth_client_id: string;
    oauth_secret_key: SecretSetting;
    login_mode: 'allowlist' | 'auto' | string;
    login_accounts: string[];
    login_auto_role: Role;
    oauth_revoke_after_login: boolean;
  };
  smtp: {
    host: string;
    port: number;
    user: string;
    password: SecretSetting;
    from: string;
    security: 'none' | 'starttls' | 'tls' | string;
  };
  license: { disable_on_refund: boolean };
  download: { limit_per_day: number; url_ttl_seconds: number };
  checkout: { success_url: string; cancel_url: string };
}

/** Partial update body for `PUT /settings`. Secret fields: string = set, omitted = keep, "" = clear. */
export interface SettingsUpdate {
  stripe?: {
    secret_key?: string;
    webhook_secret?: string;
    managed_payments?: boolean;
    oauth_client_id?: string;
    oauth_secret_key?: string;
    login_mode?: string;
    login_accounts?: string[];
    login_auto_role?: Role;
    oauth_revoke_after_login?: boolean;
  };
  smtp?: {
    host?: string;
    port?: number;
    user?: string;
    password?: string;
    from?: string;
    security?: string;
  };
  license?: { disable_on_refund?: boolean };
  download?: { limit_per_day?: number; url_ttl_seconds?: number };
  checkout?: { success_url?: string; cancel_url?: string };
}

/* --------------------------------------------------------------- imports */

export type ImportStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export interface ImportProgress {
  stage: string;
  done: number;
  total: number;
  counts: Record<string, number>;
  warnings: number;
}

export interface ImportJob {
  id: ID;
  kind: string;
  status: ImportStatus;
  options: Record<string, unknown> | null;
  progress: ImportProgress | null;
  error: string | null;
  created_by: string | null;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
}

export interface ImportLog {
  id: ID;
  level: 'info' | 'warn' | 'warning' | 'error' | string;
  message: string;
  created_at: string;
}

/* ------------------------------------------------------------ storefront */

export interface StorefrontOrderSummary {
  identifier: string;
  order_number: number;
  created_at: string;
  status: string;
  total_formatted: string;
  currency: string;
  items: { product_name: string; variant_name: string }[];
  license_key_count: number;
  file_count: number;
}

export interface StorefrontSubscriptionSummary {
  id: ID;
  status: string;
  status_formatted: string;
  product_name: string;
  variant_name: string;
  renews_at: string | null;
  ends_at: string | null;
  billing_url: string | null;
}

export interface StorefrontMe {
  email: string;
  orders: StorefrontOrderSummary[];
  subscriptions: StorefrontSubscriptionSummary[];
}

export interface StorefrontOrder {
  identifier: string;
  order_number: number;
  status: string;
  status_formatted: string;
  created_at: string;
  user_name: string;
  user_email: string;
  currency: string;
  subtotal_formatted: string;
  discount_total_formatted: string;
  tax_formatted: string;
  total_formatted: string;
  refunded: boolean;
  card_brand: string | null;
  card_last_four: string | null;
  items: { product_name: string; variant_name: string; quantity: number; price_formatted: string }[];
  license_keys: {
    key: string;
    status: string;
    status_formatted: string;
    activation_limit: number | null;
    instances_count: number;
    expires_at: string | null;
    product_name: string;
    variant_name: string;
  }[];
  files: {
    id: ID;
    name: string;
    extension: string;
    size_formatted: string;
    version: string | null;
    product_name: string;
    variant_name: string;
    download_url: string;
  }[];
  links: { title: string; url: string }[];
  subscriptions: {
    id: ID;
    status: string;
    status_formatted: string;
    product_name: string;
    renews_at: string | null;
    ends_at: string | null;
  }[];
  billing_url: string | null;
  invoice_url: string | null;
}
