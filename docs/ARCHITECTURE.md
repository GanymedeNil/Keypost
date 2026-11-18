# Architecture

## 1. Components

| Component | Path | Responsibility |
| --- | --- | --- |
| API server | `backend/cmd/server` | LS-compatible API, License API, Stripe webhook, customer API, Admin API, downloads, background jobs |
| Ops CLI | `backend/cmd/dfctl` | `migrate`, `admin create / reset-password / reset-mfa`, `apikey create` (operates directly on the database) |
| Web | `frontend/` | Next.js: `/admin/*` admin UI, `/my-orders/*` customer pages; `rewrites` proxy to the backend |

Backend package layout (`backend/internal`):

```
config      environment variables               crypt      AES-GCM encryption, HMAC signing, random tokens
db          GORM models/migrations/connection   settings   runtime-editable config (encrypted in DB, env fallback)
urls        signed public URLs                  resource   DB models → LS JSON:API objects (shared by REST and webhooks)
lsapi       /v1 JSON:API + License API          admin      /admin-api (admin)
storefront  downloads/checkout redirect/billing portal/customer API/Stripe webhook entry point
service     business core: fulfillment, subscriptions, refunds, license keys, checkout, Stripe event dispatch
stripe      lightweight Stripe REST client + webhook signature verification + OAuth
webhooks    outgoing webhook delivery (queue, signing, retries, replay)
importer    LS data import, Stripe mapping/auto-matching
storage     local disk / S3-compatible          mail       SMTP and email templates   ratelimit / httpx / auth
```

## 2. Data flow

### Purchase (one-time)

1. The buyer creates a Stripe Checkout Session via `buy_now_url` or `POST /v1/checkouts` (`client_reference_id` = local checkout id; custom data is stored locally to avoid Stripe's 500-character metadata limit).
2. Stripe calls back `/webhooks/stripe`: verify signature (5-minute tolerance, multiple secrets supported for easy rotation) → write to `stripe_events` (event ID as primary key, so deduplication is built in) → process.
3. `checkout.session.completed` → **re-fetch** the Session (including `line_items.data.price.product`, `payment_intent.latest_charge`…) → in **one transaction**: upsert the customer, allocate the order number, write the order and its items, (subscriptions) create the subscription, issue license keys per the variant rules.
4. After the transaction commits, emit outgoing events in a fixed order: `order_created` → `subscription_created` → `subscription_payment_success` → `license_key_created`, then send the receipt email asynchronously.
5. The buyer is redirected to `/checkout/success` and waits for fulfillment (running fulfillment once synchronously if needed), then is forwarded to the signed order page or your `redirect_url` (supports variables such as `[license_key]`).

**Idempotency**: `stripe_events.id` primary key + unique index on `orders.stripe_checkout_session` + check-then-write; duplicate deliveries, concurrent deliveries, and `async_payment_succeeded` arriving together with `completed` all produce only one order. A processing failure returns 5xx so Stripe retries, and a background job also re-runs `failed` events.

### Subscription lifecycle

For every `customer.subscription.*` / `invoice.*` event, **the event payload is not trusted**; instead the latest object is re-fetched by ID and then reconciled (avoids out-of-order delivery and API version differences). `mapSubscription` maps Stripe statuses to LS statuses (`trialing→on_trial`, `cancel_at_period_end→cancelled` (grace period), `canceled→expired`, `pause_collection→paused` …), and emits `subscription_cancelled/resumed/expired/paused/unpaused` + `subscription_updated` only when the status snapshot changes. License keys bound to a subscription: expire at `ends_at` after cancellation, are revived on resume, and become invalid immediately once the subscription expires.

### Refunds

`charge.refunded` and refunds initiated from the admin/API both converge on `applyOrderRefund`, which **sets by cumulative amount** (does not add), so duplicate events have no side effects and `order_refunded` is never sent twice. A full refund disables the order's license keys by default (can be turned off).

### Outgoing webhooks

Event → for each matching endpoint, generate `webhook_deliveries` (payload is rendered and frozen, so replays are unaffected by later data changes) → a background worker claims them via lease: **pending events for the same endpoint are serialized in creation order** (the first order's `order_created` goes out before `license_key_created`), different endpoints run concurrently; as with LS, strict global ordering is not guaranteed, so consumers should treat the latest data in events such as `subscription_updated` as authoritative; failures retry with 5s/25s/125s backoff, are marked `failed` once retries are exhausted, and can be redelivered from the admin. Destination addresses reject link-local addresses (cloud metadata).

## 3. Key design points for LS compatibility

- **ID**: all primary keys are int64; import preserves the original LS IDs (including `license_keys.id`, instance `identifier`, order `identifier/order_number`), and new data continues the auto-increment (PostgreSQL sequences are reset after import).
- **Serialization**: the `resource` package generates objects in the field order of the LS docs (including `relationships` links, `first_order_item`, `urls.receipt/customer_portal`, `status_formatted`, etc.). `files` keeps the camelCase `createdAt/updatedAt` fields from the LS docs and also provides snake_case fields.
- **License key status**: derived in real time from `disabled / expires_at / instances_count` as `inactive|active|expired|disabled`; activation takes a row lock inside the transaction (PostgreSQL `FOR UPDATE`; SQLite immediate transaction) to check the limit, preventing concurrent over-issuance.
- **Test mode**: API keys carry `test_mode`, and all resource queries filter by it; data produced by Stripe test keys (`sk_test_`) has `test_mode=true`.
- **Signed URLs**: receipt `…/my-orders/{uuid}?signature=` (permanent read-only link), download `…/download/{uuid}?expires=&order=&signature=` (1 hour by default, and refund/subscription status is checked per order), billing portal `/billing?…` (24 hours; a Stripe Billing Portal session is created dynamically on access).

## 4. Security model

| Surface | Mechanism |
| --- | --- |
| Secrets at rest | Stripe keys, webhook signing secrets, the SMTP password, and outgoing webhook secrets are encrypted with AES-256-GCM using a key derived from `APP_SECRET`; the API never echoes plaintext (only `set/source/hint`) |
| Admin sessions | Random token, only its SHA-256 stored in the DB; HttpOnly + SameSite=Lax cookie; all write requests require `X-Requested-With: df-admin`; login is rate-limited per IP and per account; passwords use bcrypt(12), with an equivalent-time comparison for nonexistent accounts |
| Two-factor authentication (TOTP) | After login, every member's session (including those who "Sign in with Stripe") starts unverified and can only access `/me`, `/auth/mfa/*`, and sign-out; it becomes usable only after a TOTP (RFC 6238) or one-time recovery code. The secret is stored encrypted with `APP_SECRET`, recovery codes are stored as keyed hashes; a code from the same time window cannot be replayed; 5 consecutive failures lock the account for 15 minutes; an owner can reset others, and the last resort is `dfctl admin reset-mfa`. `MFA_REQUIRED=false` is for local development only |
| API key | `dfk_live_…/dfk_test_…`, only SHA-256 stored in the DB, shown only once at creation, can have an expiry or be revoked |
| Roles | `owner` > `admin` > `viewer` (all write operations return 403); the last owner cannot be deleted/demoted |
| Customer login | Magic link: 15 minutes, single-use, hash stored in the DB; a nonexistent email also gets 204 (prevents enumeration); rate-limited per IP/email; session cookie is HMAC-signed |
| Downloads | Signature + expiry; per-file per-IP daily limit; rejected after a refund or subscription lapse; path traversal protection for local storage |
| Stripe webhook | Constant-time signature verification, timestamp tolerance, event-level deduplication |
| Sign in with Stripe | `state` is bound to a one-time signed cookie; deauthorize immediately after login; by default only allowlisted / already linked accounts |

## 5. Stripe events to subscribe to

`checkout.session.completed`, `checkout.session.async_payment_succeeded`, `invoice.paid`, `invoice.payment_failed`,
`customer.subscription.updated|deleted|paused|resumed`, `charge.refunded`, `customer.updated`.
(Configured automatically by "Create / update webhook endpoint in Stripe" in the admin.)

## 6. Data model at a glance

`stores → products → variants → prices / files`; `customers`; `orders → order_items`; `subscriptions → subscription_items / subscription_invoices`;
`license_keys → license_key_instances`; `checkouts`; `webhooks → webhook_deliveries`; `stripe_events`; `users / admin_sessions / api_keys / audit_logs / settings`;
`customer_login_tokens / email_logs / download_logs`; `import_jobs / import_logs`.
Migrations use GORM `AutoMigrate` (runs automatically at startup; additive only, never drops).
