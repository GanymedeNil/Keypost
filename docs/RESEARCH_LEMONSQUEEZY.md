# Lemon Squeezy Digital Fulfillment Research

> Research date: 2026-11-17. Source: the migration / API / help pages on docs.lemonsqueezy.com (raw HTML extracted and cross-checked; fields follow the official examples).

## 1. Background

- Lemon Squeezy has stopped accepting new users and will **stop processing transactions on 2027-03-31** (including subscription renewals), with no extension.
- Officially recommends migrating to **Stripe Managed Payments** (Stripe as MoR). But Managed Payments does **not** include:
  file delivery, download links, license keys, storefront, affiliate marketing, PayPal.
- Official wording: digital fulfillment "requires you to build it yourself or integrate a third party", and historical files / links / license keys **will not be migrated automatically**.
- The migration tool goes live on 2026-12-02: it migrates only customers (with a mapping report of LS ID → Stripe ID), products (each variant → one Stripe product),
  and card subscriptions. **Order history, license keys, files, and download entitlements must be carried over yourself.**
- All LS links / API / webhooks **will not redirect automatically**; integrators must change the addresses they call.

=> Positioning of this project: **a self-hosted fulfillment layer compatible with the LS API protocol, with data sourced from Stripe and historical data importable from LS**.
Business teams only need to replace `https://api.lemonsqueezy.com` with their own domain; the rest of their code, SDKs, and webhook handling logic should stay as untouched as possible.

## 2. Capabilities to be compatible with

### 2.1 License API (public, no authentication required)

| Item | Details |
| --- | --- |
| Base | `https://api.lemonsqueezy.com`, HTTPS only |
| Request | `Accept: application/json`; POST uses `application/x-www-form-urlencoded` |
| Rate limit | 60 requests/minute |
| Error codes | 400 (see the `error` field), 404 (not found), 422 (missing/invalid field) |

| Endpoint | Parameters | Returns |
| --- | --- | --- |
| `POST /v1/licenses/activate` | `license_key`*, `instance_name`* | `activated`, `error`, `license_key{id,status,key,activation_limit,activation_usage,created_at,expires_at}`, `instance{id(uuid),name,created_at}`, `meta{store_id,order_id,order_item_id,product_id,product_name,variant_id,variant_name,customer_id,customer_name,customer_email}` |
| `POST /v1/licenses/validate` | `license_key`*, `instance_id` | `valid`, `error`, `license_key`, `instance` (`null` when there is no instance_id), `meta` |
| `POST /v1/licenses/deactivate` | `license_key`*, `instance_id`* | `deactivated`, `error`, `license_key`, `meta` |

License key status: `inactive` (no activations) / `active` (≥1 activation) / `expired` / `disabled`.
When the activation limit is reached, it returns `activated:false` and `error:"This license key has reached the activation limit."`, and does **not** include `instance`.

### 2.2 JSON:API (Bearer authentication)

- Headers: `Accept: application/vnd.api+json`, `Content-Type: application/vnd.api+json`, `Authorization: Bearer <key>`.
- Pagination: `page[number]`, `page[size]` (default 10, max 100); the response includes `meta.page{currentPage,from,lastPage,perPage,to,total}` and `links{first,last,next,prev}`.
- Filtering: `filter[xxx]=`; relationships: `include=`; nesting: `/v1/products/100/variants`; 4xx returns `{"jsonapi":{"version":"1.0"},"errors":[{"detail","status","title"}]}`.
- Rate limit: 300 requests/minute, response headers `X-Ratelimit-Limit` / `X-Ratelimit-Remaining`, 429 when exceeded.
- Test mode: a test-mode key can only see test data; objects carry `test_mode`.

Fulfillment-related resources:

| Resource | Endpoint | Notes |
| --- | --- | --- |
| users | `GET /v1/users/me` | The user the current key belongs to |
| stores | `GET /v1/stores`, `/v1/stores/{id}` | |
| customers | `POST/GET/PATCH /v1/customers`, `GET /v1/customers/{id}` | `urls.customer_portal` is pre-signed, valid 24h |
| products | `GET /v1/products`, `/{id}` | `buy_now_url` |
| variants | `GET /v1/variants`, `/{id}` | `has_license_keys`, `license_activation_limit`, `license_length_*`, `links[]` |
| prices | `GET /v1/prices`, `/{id}` | Pricing model, subscription interval, trial |
| files | `GET /v1/files`, `/{id}` | `download_url` is signed, **expires in 1 hour, 10 downloads per IP per day** |
| orders | `GET /v1/orders`, `/{id}`; `POST /v1/orders/{id}/generate-invoice`; `POST /v1/orders/{id}/refund` | `urls.receipt` |
| order-items | `GET /v1/order-items`, `/{id}` | |
| subscriptions | `GET/PATCH/DELETE /v1/subscriptions` | `DELETE` = cancel |
| subscription-invoices | `GET /v1/subscription-invoices`, `/{id}` | |
| subscription-items | `GET/PATCH /v1/subscription-items` | |
| license-keys | `GET /v1/license-keys`, `/{id}`; `PATCH /v1/license-keys/{id}` | Can change `activation_limit` / `expires_at` / `disabled` |
| license-key-instances | `GET /v1/license-key-instances`, `/{id}` | `identifier` = `instance_id` |
| webhooks | `POST/GET/PATCH/DELETE /v1/webhooks` | `secret` is write-only |
| checkouts | `POST /v1/checkouts`, `GET /v1/checkouts/{id}` | Mapped to a Stripe Checkout Session in this project |

Out of scope: discounts / discount-redemptions / affiliates / usage-records (discounts and affiliates are left to Stripe and standalone platforms).
Imported historical orders still keep values in fields such as `discount_total` and `affiliate_id`.

### 2.3 Outgoing webhooks

- `POST` to the merchant-configured URL, `Content-Type: application/json`, headers: `X-Event-Name`, `X-Signature`.
- Signature: the **hex digest** of `HMAC-SHA256(secret, rawBody)` (constant-time comparison). Secret length 6–40.
- Body: `{"meta":{"event_name","custom_data"?}, "data":{type,id,attributes,relationships,links}}`.
- Events: `order_created`, `order_refunded`, `customer_updated`, `subscription_created|updated|cancelled|resumed|expired|paused|unpaused`,
  `subscription_payment_success|failed|recovered|refunded`, `license_key_created`, `license_key_updated`, `affiliate_activated` (not implemented).
- Event order (first order): `order_created` → `subscription_created` → `subscription_payment_success` → `license_key_created`.
- Successful renewal: `subscription_payment_success` → `subscription_updated`; failure: `subscription_payment_failed` → `subscription_updated`;
  recovery: `subscription_payment_success` + `subscription_payment_recovered` → `subscription_updated`.
- Checkout accepts `custom_data`, which is returned in `meta.custom_data` of order / subscription / license key events.

### 2.4 Customer side (My Orders / receipts)

- Customers log in to My Orders with a **magic link** sent to their order email, where they can view orders, subscriptions, **currently available files**, links, and license keys.
- After the merchant adds / replaces files, **all existing customers see them immediately**; after a file is deleted it disappears immediately.
- The receipt email contains the license key and a download entry point; link variables: `[license_key] [order_id] [order_identifier] [email] [name] [total]`.
- After a subscription expires, its license key expires with it; the validity of a subscription license is tied to the subscription lifecycle.
- Order export CSV columns: `identifier, order_number, product_name, variant_name, user_name, user_email, city, country, postal_code,
  card_brand, card_last_four, currency, currency_rate, subtotal, discount_total, tax, total, tax_name, tax_rate, tax_number, date_utc`.

### 2.5 License key semantics

- On a variant: `has_license_keys`, `license_activation_limit` (or unlimited), `license_length_value/unit(days|months|years)` (or unlimited).
- Subscription variant: key expiry = subscription expiry (expires after cancellation takes effect, or when dunning ends).
- Full refund: the license key is set to disabled.
- Key format: UUID v4 (example: `38b1460a-5104-4067-a91d-77b872934d51`); `key_short` = `XXXX-` + the last 12 characters.

## 3. Stripe-side conventions

- Fulfillment events: `checkout.session.completed` (`payment_status=paid|no_payment_required`), and for delayed payments etc. `checkout.session.async_payment_succeeded`;
  `line_items` must be fetched via the API (the webhook does not include them), and fulfillment must be idempotent.
- Subscriptions: `invoice.paid`, `invoice.payment_failed`, `customer.subscription.updated|deleted`. Refunds: `charge.refunded`. Customers: `customer.updated`.
- API version differences (since basil): `current_period_end` moved from Subscription to `items.data[].current_period_end`;
  Invoice's `subscription` moved to `parent.subscription_details.subscription`. **Parsing must support both shapes**.
- Managed Payments: pass `managed_payments[enabled]=true` when creating a Checkout Session (requires a Preview API version, and is incompatible with Connect).
- Stripe login: Connect OAuth (`connect.stripe.com/oauth/authorize` → `/oauth/token` returns `stripe_user_id`).
  The platform needs the `read_write` scope; Stripe does not recommend OAuth for new platforms, but it is the only official "account identity" login flow. This project calls `deauthorize` immediately after login and keeps only the identity.

## 4. Mapping to this project

| LS concept | This project |
| --- | --- |
| Store | `stores` (multi-store supported; a login account can be bound) |
| Product / Variant / Price | LS IDs retained; `variants.stripe_product_id` / `prices.stripe_price_id` map to Stripe |
| File | Local disk / S3-compatible storage; signed download URL (1h, 10 per day per IP) |
| Customer | LS ID retained; `customers.stripe_customer_id` is imported from the migration mapping CSV (LS ID → Stripe ID) |
| Order / OrderItem | From Stripe `checkout.session.completed`; historical orders imported from LS |
| Subscription | From Stripe; after import, historical subscriptions are linked via the `stripe_subscription_id` mapping |
| License key / instance | **Imported as-is** (key strings and instance UUIDs unchanged); existing clients need no key changes |
| Webhook | Same event names, same signature algorithm, same payload shape |
| Customer Portal | `/billing` pre-signed → dynamically creates a Stripe Billing Portal Session |
| Checkout | `POST /v1/checkouts` → Stripe Checkout Session; `buy_now_url` → `/checkout/buy/{variant_slug}` |
