# Compatibility with the Lemon Squeezy API

Baseline: docs.lemonsqueezy.com (fetched 2026-11). **Replace `https://api.lemonsqueezy.com` with `https://<your-domain>`**; the official SDKs (`@lemonsqueezy/lemonsqueezy.js`, etc.) work through a custom base URL.

## License API (no authentication, `application/x-www-form-urlencoded`, JSON also accepted)

| Endpoint | Status | Notes |
| --- | --- | --- |
| `POST /v1/licenses/activate` | ✅ | Fields, `meta`, and error messages (`This license key has reached the activation limit.`, etc.) match; over-limit/disabled/expired returns 400 with `license_key` and `meta`, without `instance` |
| `POST /v1/licenses/validate` | ✅ | With no (or empty) `instance_id`, `instance: null`; looks up the instance first, then checks disabled/expired; `license_key` includes `test_mode` |
| `POST /v1/licenses/deactivate` | ✅ | Releases one activation slot; `license_key.activation_usage` updates immediately |
| Parameter validation | ✅ | Laravel-style 422: `{"message":"The instance name field must be at least 3 characters.","errors":{"instance_name":[…]}}` (`license_key` required; `instance_name` required, 3–255 characters; `deactivate` requires `instance_id`; with multiple errors the message ends with ` (and N more error(s))`). Validation runs before the key lookup |
| 404 | ✅ | Key not found: `license_key not found.`; instance not found: `instance_id not found.`; the response body has **only** two fields, `activated`/`valid`/`deactivated` and `error` (no `license_key`, `meta`, or `instance`). The expired message is `This license key is expired.` |
| Rate limiting | ✅ | 60 per minute per IP (`LICENSE_RATE_LIMIT`), responses carry `X-Ratelimit-*`, 429 when exceeded |

## JSON:API (`Authorization: Bearer <key>`, `Accept: application/vnd.api+json`)

| Resource | Read | Write | Notes |
| --- | --- | --- | --- |
| `GET /v1/users/me` | ✅ | | |
| stores | ✅ | ❌ | Create/edit in the admin UI; stats such as `total_sales` are computed live and **count only live orders** (same as LS, test orders are excluded; likewise customers' `total_revenue_currency`) |
| customers | ✅ | ✅ create / update | `urls.customer_portal` = 24h signed link; a Stripe Billing Portal session is created dynamically on access |
| products / variants / prices | ✅ | ❌ | Same as LS, the catalog is maintained in the admin UI; `buy_now_url` points to this service's checkout entry. A variant can have multiple price records (changing a price in LS adds a record); the variant/product `price` and `interval` take the **latest** one; for one-time prices, `tiers` and `setup_fee_enabled` are `null` |
| files | ✅ | ❌ | `download_url` is signed for 1 hour, 10 per IP per day (configurable); file objects contain both `createdAt` and `created_at` |
| orders | ✅ | ✅ refund, generate-invoice | refund calls Stripe Refund; invoice returns the Stripe receipt/invoice link, otherwise this service's receipt page |
| order-items | ✅ | | |
| subscriptions | ✅ | ✅ `PATCH cancelled/pause`, `DELETE` | Cancel = Stripe `cancel_at_period_end`; for plan changes/usage, use Stripe |
| subscription-invoices | ✅ | ✅ refund | |
| subscription-items | ✅ | ❌ | |
| license-keys | ✅ | ✅ `PATCH activation_limit/expires_at/disabled` | Triggers `license_key_updated` |
| license-key-instances | ✅ | | `identifier` is the `instance_id` |
| webhooks | ✅ | ✅ create / update / delete | `secret` 6–40 characters, write-only |
| checkouts | ✅ | ✅ create | Creates a Stripe Checkout Session; `url` is the Stripe-hosted page; supports `custom_price`, `checkout_data.{email,name,custom,variant_quantities,billing_address,discount_code→allow_promotion_codes}`, `product_options.redirect_url`, `expires_at` |
| discounts / discount-redemptions / usage-records / affiliates | ❌ | ❌ | Return JSON:API 404; for discounts, use Stripe Promotion Codes |

Common behavior: `page[number]`/`page[size]` (default 10, max 100), `meta.page`, `links.first/last/next/prev`, `filter[...]`, `include=`, `sort=`, nested routes (`/v1/orders/1/license-keys`, `/v1/orders/1/customer` …), error body `{"jsonapi":…, "errors":[{detail,status,title}]}` (validation errors carry `source.pointer`), rate limit 300 per minute + `X-Ratelimit-Limit/Remaining`.

## Webhook (outgoing)

| Item | Status |
| --- | --- |
| Events | `order_created` `order_refunded` `customer_updated` `subscription_created/updated/cancelled/resumed/expired/paused/unpaused` `subscription_payment_success/failed/recovered/refunded` `license_key_created/updated` (`affiliate_activated` can be subscribed to but never fires) |
| Headers | `Content-Type: application/json`, `X-Event-Name`, `X-Signature` (`HMAC_SHA256(secret, rawBody)` hex; use the Node/PHP/Python signature-verification code from the LS docs as is) |
| Body | `{"meta":{"event_name","custom_data"},"data":{type,id,attributes,relationships,links}}` |
| Event order | First order: `order_created → subscription_created → subscription_payment_success → license_key_created`; successful renewal: `subscription_payment_success → subscription_updated`; failure/recovery/cancellation/expiry sequences match the LS docs |
| Retries | Non-2xx or timeout (10s) is retried, 4 attempts in total by default (5s/25s/125s backoff) |
| Custom data | `checkout_data.custom` → `meta.custom_data` of order, subscription, license key, and invoice events (for Stripe Checkouts not created by this service, taken from the Session metadata) |

## Intentional differences

1. **Stripe is the payment processor**: `payment_processor` is always `stripe`; historical PayPal subscriptions are imported but cannot renew (LS itself doesn't support migrating PayPal subscriptions either).
2. **Amount basis**: `subtotal` = Stripe `amount_subtotal`, `discount_total`/`tax` come from `total_details`, `total` = `amount_total`; `*_usd` are approximate conversions.
3. **Order numbers**: this service auto-increments per store; imported historical orders keep their original order numbers, and new orders continue from the maximum.
4. **Download link domain**: changes from `app.lemonsqueezy.com` to your own domain; the link embeds order information and becomes invalid immediately after a refund or once the subscription lapses.
5. **Customer portal**: `urls.customer_portal` is no longer the LS Portal but redirects to the Stripe Billing Portal (subscriptions, payment methods, invoices).
6. **IDs and keys**: imported data keeps its IDs; newly generated license keys are still UUID v4.

## What was compared against real Lemon Squeezy

The License API's response shapes, status codes, and error messages, plus all attributes of stores / customers / products / variants / prices / orders / order-items / license-keys / license-key-instances in the JSON:API,
were compared field by field against a real test-mode LS account: the importer loaded the account's data into this service, then the `/v1/*` data from both sides was diffed, and the same key was run through activate / over-limit / validate / deactivate / disable / expire and similar operations on both sides and the results compared.
The only remaining differences are URLs that differ by design (`buy_now_url`, `urls.receipt`, and `customer_portal` point to your own domain). **Resources with no real samples** — subscriptions, subscription invoices, files — are still implemented per the official docs and have not been compared.
