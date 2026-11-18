# Admin API contract (`/admin-api/*`) and customer API (`/storefront-api/*`)

The frontend (Next.js) and backend (Go) both follow this file. **The backend implements it, the frontend consumes it.** Changes must be synced on both sides.

## 0. General conventions

- Base: same-origin, path prefix `/admin-api`. Next.js `rewrites` proxy `/admin-api/*`, `/storefront-api/*`, `/v1/*`, `/download/*`, `/billing`, `/checkout/*`, `/webhooks/*`, `/healthz` to Go (`BACKEND_URL`, default `http://localhost:8080`).
- Auth: after login the server sets an HttpOnly cookie `df_admin`. **All POST/PUT/PATCH/DELETE must carry the request header `X-Requested-With: df-admin`** (CSRF protection), otherwise 403.
- Not signed in → `401 {"error":{"code":"unauthenticated",...}}`; insufficient permission → `403 {"error":{"code":"forbidden"}}`.
- Roles: `owner` (everything) > `admin` (everything except "user management / API key creation and revocation / settings writes") > `viewer` (read-only, writes get 403; **exception**: `PATCH /me`, `POST /me/password`, `DELETE /me/stripe` are callable by any role). See §8.
- Requests/responses are all `application/json` (except uploads: `multipart/form-data`). Fields are snake_case. Times are RFC3339 UTC (`2026-11-17T09:30:00Z`); amounts are integers (smallest currency unit, cents).
- Errors: `{"error":{"code":"validation_failed","message":"…","fields":{"email":"is invalid"}}}`; common codes: `unauthenticated forbidden not_found validation_failed conflict stripe_error rate_limited internal`.
- Lists: query parameters `page` (default 1), `per_page` (default 25, max 100), `q` (search), `sort` (e.g. `-created_at`; default varies per resource), `test_mode=true|false` (default `false`), `store_id`, and each resource's own filters. Response:

```json
{"data":[ ... ],"meta":{"page":1,"per_page":25,"total":132,"last_page":6}}
```

- Single resource: the object is returned directly (not wrapped in `data`). Successful delete: `204`.
- `ID`s are always numeric (JSON number).

## 1. Authentication

| Method Path | Description |
| --- | --- |
| `GET /auth/config` | No login required. `{"password_login":true,"stripe_login":true,"setup_required":false}` |
| `POST /auth/setup` | Creates the owner when no users exist: `{"setup_token","email","name","password"}` (`setup_token` is printed in the log on the service's first start, or bootstrap with the `ADMIN_EMAIL/ADMIN_PASSWORD` env vars). Logs in automatically on success |
| `POST /auth/login` | `{"email","password"}` → `{"user":User}` and Set-Cookie |
| `POST /auth/logout` | 204 |
| `GET /auth/stripe/start?mode=login` | Full-page browser redirect (302) to the Stripe authorization page; `mode=link` (requires being signed in) links the Stripe account to the current user |
| `GET /auth/stripe/callback` | Stripe callback (handled by the backend). On success 302 → `/admin` (link mode → `/admin/settings/profile?linked=1`); on failure 302 → `/admin/login?error=<code>`. Possible `error` values: `stripe_denied stripe_state stripe_not_allowed stripe_exchange disabled` |
| `GET /me` | `User` |
| `PATCH /me` | `{"name"}` |
| `POST /me/password` | `{"current_password","new_password"}` (≥10 characters) |
| `DELETE /me/stripe` | Unlinks the Stripe account (a password must already be set, otherwise 409) |

`User`: `{id,email,name,role,stripe_account_id,stripe_account_label,disabled,has_password,mfa_enabled,last_login_at,created_at}`; `/me`, `/auth/login`, `/auth/setup` and `/auth/mfa/verify` additionally include `mfa` (see §1.1).

The login page should offer two methods: an email/password form, and a "Sign in with Stripe" button (when `stripe_login=true`, linking to `/admin-api/auth/stripe/start?mode=login`).

### 1.1 Mandatory two-factor authentication (MFA, TOTP)

When `MFA_REQUIRED` (env var, default `true`) is true, **all members** — whether they sign in with email/password or "Sign in with Stripe" — get an **unverified** session after every login and must pass one more TOTP (or recovery code) before using the back office. There is no switch for members to turn MFA off themselves.

- **An unverified session can only call**: `GET /me`, `POST /auth/logout`, `/auth/mfa/*`. Every other `/admin-api/*` returns `403`: `{"error":{"code":"mfa_setup_required"|"mfa_required","message":"…"}}` (authenticator already set up → `mfa_required`, not yet set up → `mfa_setup_required`). The frontend redirects to `/admin/mfa` based on this.
- API keys on `/v1/*` are machine credentials and are not affected.
- The `User` returned by `GET /me` and `POST /auth/login` (and `/auth/setup`) has an extra `mfa`: `{"required":true,"enabled":false,"verified":false,"recovery_codes_left":0}`; `enabled` = an authenticator is set up, `verified` = the **current session** has passed verification. `users` list items carry only `mfa_enabled`.
- Algorithm: RFC 6238, SHA-1, 6 digits, 30 seconds, accepting one time window on either side; a code from the same time window cannot be reused. The secret is stored in the database encrypted with `APP_SECRET`; recovery codes are stored hashed only.

| Method Path | Description |
| --- | --- |
| `POST /auth/mfa/setup` | Starts setup: generates a new secret (overwriting any earlier unfinished one). Already set up → `409`. Returns `{"secret":"BASE32…","otpauth_uri":"otpauth://totp/…"}`; the frontend renders `otpauth_uri` as a QR code and also shows `secret` for manual entry |
| `POST /auth/mfa/enable` | `{"code":"123456"}`; once verified, MFA is formally enabled and the session becomes verified; returns `{"recovery_codes":["xxxxx-xxxxx",…10 codes]}` — shown **this once only**. Wrong code → `422` (`fields.code`) |
| `POST /auth/mfa/verify` | `{"code":"123456"}` or `{"recovery_code":"xxxxx-xxxxx"}`; once passed, the session becomes verified and `User` is returned. 5 consecutive wrong attempts lock the account for 15 minutes (`429 rate_limited`, `message` includes the remaining minutes); recovery codes are single-use |
| `POST /me/mfa/recovery-codes` | Verified session + `{"code":"123456"}` → invalidates the old recovery codes and returns 10 new ones `{"recovery_codes":[…]}` |
| `POST /users/{id}/mfa/reset` | **owner**: clears the member's authenticator and recovery codes and invalidates all of their sessions; they set up again at next login (for a new phone/lost device). Cannot be used on yourself (`409`), audit `user.mfa_reset` |

Last resort (e.g. the only owner lost their device): run `dfctl admin reset-mfa --email you@example.com` on the server. Audit actions: `auth.mfa_enabled`, `auth.mfa_verified`, `auth.mfa_failed`, `auth.mfa_recovery_used`, `auth.mfa_recovery_regenerated`, `user.mfa_reset`.

## 2. Dashboard

`GET /dashboard?store_id=&test_mode=&days=30`

```json
{
  "currency":"USD",
  "totals":{"revenue":1234500,"orders":321,"refunded":12000,"customers":210,"active_subscriptions":55,"mrr":99000,"license_keys":300,"active_license_keys":120,"downloads":4021},
  "previous":{"revenue":1000000,"orders":280},
  "series":[{"date":"2026-11-01","revenue":10000,"orders":3,"refunds":0}],
  "recent_orders":[Order],
  "recent_events":[{"kind":"stripe_event|webhook_delivery|import","title":"checkout.session.completed","status":"processed","at":"…"}],
  "health":{"stripe_configured":true,"stripe_webhook_secret_set":true,"smtp_configured":true,"failed_stripe_events":0,"failed_webhook_deliveries":2,"pending_webhook_deliveries":0,"storage_driver":"local"}
}
```

## 3. Resources

In what follows, `[]` means optional; `R` = read-only list + detail, `CRUD` means `GET list / POST / GET one / PATCH / DELETE`.

### stores — read: any signed-in role; write: owner/admin
`Store`: `{id,name,slug,domain,url,avatar_url,plan,country,currency,support_email,email_from_name,receipt_subject,receipt_intro,stripe_account_id,total_sales,total_revenue,thirty_day_sales,thirty_day_revenue,created_at,updated_at}`
POST/PATCH fields: `name,slug,domain,url,avatar_url,country,currency,support_email,email_from_name,receipt_subject,receipt_intro`.

### products — CRUD
List filters: `status`, `q`. `Product`: `{id,store_id,name,slug,description,status(draft|published),thumb_url,large_thumb_url,stripe_product_id,test_mode,buy_now_url,variants_count,from_price,to_price,created_at,updated_at}`
Detail adds `variants:[Variant]`. POST/PATCH: `store_id,name,slug,description,status,thumb_url,large_thumb_url,stripe_product_id,test_mode`. DELETE cascades to variants/prices/files (requires no order references first, otherwise 409).

### variants — CRUD
Filter: `product_id`. `Variant`:
`{id,product_id,product_name,name,slug,description,links:[{title,url}],sort,status(pending|draft|published),has_license_keys,license_activation_limit,is_license_limit_unlimited,license_length_value,license_length_unit(days|months|years),is_license_length_unlimited,stripe_product_id,buy_now_url,prices:[Price],files_count,test_mode,created_at,updated_at}`
Detail adds `files:[File]`. On POST you can also pass `price:{…Price fields}` to create one at the same time.

### prices — CRUD
Filter: `variant_id`. `Price`: `{id,variant_id,category(one_time|subscription|lead_magnet|pwyw),scheme(standard|package|graduated|volume),unit_price,unit_price_decimal,setup_fee_enabled,setup_fee,package_size,tiers:[{last_unit,unit_price,unit_price_decimal,fixed_fee}],renewal_interval_unit(day|week|month|year),renewal_interval_quantity,trial_interval_unit,trial_interval_quantity,min_price,suggested_price,tax_code,currency,stripe_price_id,created_at,updated_at}`

### files
- `GET /files?variant_id=&status=` list; `GET /files/{id}`; `PATCH /files/{id}` (`name,version,sort,status`); `DELETE /files/{id}`.
- `POST /variants/{id}/files`: `multipart/form-data`, fields `file` (required), `name`, `version`, `status` (default published). Returns `File`. The upload size limit is set by the backend's `MAX_UPLOAD_MB` (default 1024).
- `GET /files/{id}/download`: direct download for admins (cookie auth).
- `File`: `{id,variant_id,variant_name,product_name,identifier,name,extension,size,size_formatted,version,sort,status(draft|published),storage_driver,content_type,sha256,downloads_count,test_mode,created_at,updated_at}`

### customers — R + PATCH
Filters: `q` (name/email), `country`. `Customer`: `{id,store_id,name,email,status,city,region,country,stripe_customer_id,total_revenue,mrr,orders_count,subscriptions_count,license_keys_count,test_mode,created_at,updated_at}`
Detail adds `orders:[Order]`, `subscriptions:[Subscription]`, `license_keys:[LicenseKey]`.
`PATCH`: `name,email,city,region,country,stripe_customer_id`.
`POST /customers/{id}/portal-link` → `{"url":"https://billing.stripe.com/…"}` (Stripe Billing Portal session).
`POST /customers` creates manually: `store_id,name,email`.

### orders — R + actions
Filters: `status`, `customer_id`, `q` (order number/email/name), `created_from`, `created_to` (RFC3339). Default sort `-created_at`.
`Order`: `{id,store_id,customer_id,identifier,order_number,user_name,user_email,currency,currency_rate,subtotal,setup_fee,discount_total,tax,total,refunded_amount,subtotal_usd,discount_total_usd,tax_usd,total_usd,refunded_amount_usd,tax_name,tax_rate,tax_inclusive,status(pending|failed|paid|refunded|partial_refund|fraudulent),refunded,refunded_at,affiliate_id,referral_amount,country,city,postal_code,card_brand,card_last_four,custom_data,stripe_checkout_session,stripe_payment_intent,stripe_invoice_id,stripe_customer_id,source(stripe|lemonsqueezy|manual),test_mode,receipt_url,first_item:{product_name,variant_name,price,quantity},created_at,updated_at}`
Detail adds `items:[OrderItem]`, `license_keys:[LicenseKey]`, `subscriptions:[Subscription]`, `files:[File]` (files currently downloadable for this order), `emails:[EmailLog]`, `webhook_deliveries:[Delivery]`.
`OrderItem`: `{id,order_id,product_id,variant_id,price_id,product_name,variant_name,price,quantity}`
- `POST /orders/{id}/refund`: `{"amount":1000?,"reason":"requested_by_customer"?}`; omitting amount = full refund. Calls Stripe Refund; on success the local status is updated by the webhook (and also optimistically locally).
- `POST /orders/{id}/resend-receipt`: `{"email"?}` → 204.
- `GET /orders/export.csv?…same filters as the list`: CSV download (columns same as the LS export).

### subscriptions — R + actions
Filters: `status`, `customer_id`, `product_id`, `q`. `Subscription`: `{id,store_id,customer_id,order_id,order_item_id,product_id,variant_id,price_id,product_name,variant_name,user_name,user_email,status(on_trial|active|paused|past_due|unpaid|cancelled|expired),status_formatted,card_brand,card_last_four,payment_processor,pause,cancelled,trial_ends_at,billing_anchor,quantity,renews_at,ends_at,stripe_subscription_id,stripe_customer_id,test_mode,created_at,updated_at}`
Detail adds `invoices:[SubscriptionInvoice]`, `license_keys:[LicenseKey]`.
- `POST /subscriptions/{id}/cancel` (Stripe: `cancel_at_period_end=true`); `POST /subscriptions/{id}/resume`; `POST /subscriptions/{id}/sync` (re-fetches from Stripe and reconciles); `POST /subscriptions/{id}/link-stripe`: `{"stripe_subscription_id"}` (manual hookup after migration).
- `SubscriptionInvoice`: `{id,subscription_id,customer_id,user_name,user_email,billing_reason,card_brand,card_last_four,currency,status(pending|paid|void|refunded|partial_refund),refunded,refunded_at,subtotal,discount_total,tax,total,refunded_amount,invoice_url,stripe_invoice_id,created_at}`; `GET /subscription-invoices?subscription_id=`.

### license-keys
Filters: `status` (inactive|active|expired|disabled), `order_id`, `customer_id`, `product_id`, `variant_id`, `q` (key / email).
`LicenseKey`: `{id,store_id,customer_id,order_id,order_item_id,product_id,variant_id,subscription_id,product_name,variant_name,user_name,user_email,key,key_short,activation_limit(null=unlimited),instances_count,disabled,status,status_formatted,expires_at,test_mode,created_at,updated_at}`; detail adds `instances:[Instance]`.
- `PATCH /license-keys/{id}`: `{"activation_limit":5|null,"expires_at":"…"|null,"disabled":true|false}`
- `POST /license-keys`: manual issuance `{"order_id"?, "customer_id"?, "variant_id", "user_email"?, "user_name"?, "activation_limit"?, "expires_at"?}` (without an order, `customer_id` or `user_email` is required).
- `POST /license-keys/{id}/regenerate`: regenerates the key string and clears the activation instances.
- `GET /license-key-instances?license_key_id=`; `DELETE /license-key-instances/{id}` (frees one activation slot).
- `Instance`: `{id,license_key_id,identifier,name,created_at,updated_at}`

### webhooks (outgoing, for the merchant's business systems) — CRUD
`Webhook`: `{id,store_id,url,events:[string],disabled,secret_set,last_sent_at,test_mode,stats:{success_24h,failed_24h},created_at,updated_at}`; POST/PATCH fields: `store_id,url,events,secret(6–40 characters, write-only),disabled,test_mode`.
- `GET /webhook-events` → `["order_created",…]` (enumeration of selectable events).
- `POST /webhooks/{id}/test`: sends a sample `order_created` and returns the corresponding `Delivery` (waits synchronously).
- `GET /webhook-deliveries?webhook_id=&status=&event_name=`; `GET /webhook-deliveries/{id}` (adds a `payload` string); `POST /webhook-deliveries/{id}/redeliver`.
- `Delivery`: `{id,webhook_id,webhook_url,event_name,resource_type,resource_id,status(pending|success|failed),attempts,next_attempt_at,last_status_code,last_response,last_error,duration_ms,created_at,delivered_at}`

### stripe-events — R
`GET /stripe-events?status=&type=&q=`; `GET /stripe-events/{id}` (includes `payload`); `POST /stripe-events/{id}/replay` (reprocess).
`StripeEvent`: `{id,type,account,livemode,status(received|processed|ignored|failed),attempts,error,received_at,processed_at}`

### api-keys (Bearer keys used by the LS-compatible `/v1/*`) — list: owner/admin; create/revoke: owner
`GET /api-keys`; `POST /api-keys` `{"name","test_mode","expires_at"?}` → `{...APIKey,"token":"dfk_live_…"}` (**the token is returned this once only**); `DELETE /api-keys/{id}` (revoke).
`APIKey`: `{id,name,prefix,test_mode,owner_email,last_used_at,expires_at,revoked_at,created_at}`

### users (back-office members) — owner
CRUD: `POST {"email","name","role","password"?,"stripe_account_id"?}`; `PATCH {name,role,disabled,password?,stripe_account_id?}`; the last owner cannot be deleted or demoted.

### audit-logs — R
`GET /audit-logs?action=&user_email=&resource_type=` → `{id,user_email,action,resource_type,resource_id,details,ip,created_at}`

### email-logs — R
`GET /email-logs?status=&q=` → `{id,order_id,to,subject,template,status,error,created_at}`; `POST /email-logs/test` `{"to"}` sends a test email.

## 4. Stripe integration

- `GET /stripe/status` → `{configured,livemode,account:{id,business_name,country,email},webhook_secret_set,webhook_url,required_events:[…],error?}`
- `POST /stripe/webhook-endpoint`: calls the Stripe API to create/update a webhook endpoint at `webhook_url` (required events), saves the signing secret to settings, and returns `{id,url,events,created}`.
- `GET /stripe/catalog` → Stripe Products + Prices: `[{id,name,active,description,metadata,default_price,prices:[{id,unit_amount,currency,recurring:{interval,interval_count}|null,active}],mapped_variant_id|null}]`
- `POST /stripe/catalog/import`: `{"store_id","stripe_product_ids":[…]?,"publish":true}` builds the Stripe catalog into Product/Variant/Price (including the Stripe mapping).
- `POST /variants/{id}/link-stripe`: `{"stripe_product_id","stripe_price_id"}`; `DELETE /variants/{id}/link-stripe`.

## 5. Settings

`GET /settings` (owner/admin; secret fields are never echoed back in plaintext)

```json
{
  "urls":{"public_url":"https://pay.example.com","stripe_webhook_url":"https://pay.example.com/webhooks/stripe","stripe_oauth_redirect_uri":"https://pay.example.com/admin-api/auth/stripe/callback","api_base_url":"https://pay.example.com/v1"},
  "stripe":{"secret_key":{"set":true,"source":"db","hint":"sk_live_…1234"},"webhook_secret":{"set":true,"source":"env","hint":"whsec_…ab12"},"api_version":"2025-09-30.clover","managed_payments":false,
            "oauth_client_id":"ca_…","oauth_secret_key":{"set":false},"login_mode":"allowlist","login_accounts":["acct_123"],"login_auto_role":"viewer","oauth_revoke_after_login":true},
  "smtp":{"host":"","port":587,"user":"","password":{"set":false},"from":"","security":"starttls"},
  "license":{"disable_on_refund":true},
  "download":{"limit_per_day":10,"url_ttl_seconds":3600},
  "checkout":{"success_url":"","cancel_url":""}
}
```

`PUT /settings` (owner): **partial** update with the same structure. For secret fields, a string = set, omitted = keep, `""` = clear. Returns the same as GET.
`POST /settings/smtp/test` `{"to"}`.

## 6. Data migration (imports)

- `GET /imports`; `GET /imports/{id}`; `GET /imports/{id}/logs?after=<logId>` (incremental logs, `[{id,level,message,created_at}]`); `POST /imports/{id}/cancel`.
- `ImportJob`: `{id,kind,status(pending|running|completed|failed|cancelled),options,progress:{stage,done,total,counts:{products:3,…},warnings:2},error,created_by,started_at,finished_at,created_at}`
- `POST /imports/lemonsqueezy` (pulls from the LS API): `{"api_key","test_mode":false,"store_ids":[…]?,"resources":["stores","products","variants","prices","files","customers","orders","subscriptions","license_keys"]?,"download_files":true,"dry_run":false,"on_conflict":"abort"}` → `ImportJob` (async). `on_conflict`: `"abort"` (default) stops when it hits "ID already taken by a different record in this database" and ends as `failed`; `"overwrite"` overwrites local data (with a warning per row). Any other value → 422 `/on_conflict`. A dry run performs the same check, lists all conflicts, and ends as `failed`. See `docs/MIGRATION.md` §0.
- `POST /imports/lemonsqueezy/archive` (offline import): `multipart`, fields `file` (ZIP containing a paginated JSON:API export of each resource, see `docs/MIGRATION.md`), `download_files`, `dry_run`, `test_mode`, `on_conflict` (same as above).
- `POST /imports/stripe-mapping`: `multipart`, field `file` (the "LS Customer ID → Stripe Customer ID" CSV produced by the LS migration tool), optional `kind=customers|products|subscriptions` (default customers).
- `POST /imports/stripe-auto-match`: `{"customers":true,"products":true,"subscriptions":true}` auto-matches via the Stripe API by email / product name; unmatched items are written to the log.
- `GET /imports/mapping-report.csv`: the list of customers / variants / subscriptions that currently have no Stripe mapping (CSV download).

## 7. Customer-facing (public) API `/storefront-api/*` (no admin login required)

Cookie `df_customer` (HttpOnly, 30 days). POST also requires `X-Requested-With: df-admin`.

| Method Path | Description |
| --- | --- |
| `POST /auth/magic-link` | `{"email"}` → always 204 (does not reveal whether the email exists); sends `/my-orders/verify?token=…` to that email (valid 15 minutes, single-use) |
| `POST /auth/verify` | `{"token"}` → `{"email"}` and Set-Cookie; invalid → 400 `invalid_token` |
| `POST /auth/logout` | 204 |
| `GET /me/orders` | Requires cookie. `{"email","orders":[{identifier,order_number,created_at,status,total_formatted,currency,items:[{product_name,variant_name}],license_key_count,file_count}],"subscriptions":[{id,status,status_formatted,product_name,variant_name,renews_at,ends_at,billing_url}]}` |
| `GET /orders/{identifier}` | Requires cookie (email must match) **or** the `signature` query parameter (receipt link). See below |
| `GET /billing-url` | Requires cookie → `{"url"}` (Stripe Billing Portal) |

`GET /orders/{identifier}` returns:

```json
{
  "identifier":"…","order_number":1042,"status":"paid","status_formatted":"Paid","created_at":"…",
  "user_name":"…","user_email":"…","currency":"USD","subtotal_formatted":"$9.99","discount_total_formatted":"$0.00","tax_formatted":"$2.00","total_formatted":"$11.99","refunded":false,
  "card_brand":"visa","card_last_four":"4242",
  "items":[{"product_name":"…","variant_name":"…","quantity":1,"price_formatted":"$9.99"}],
  "license_keys":[{"key":"…","status":"active","status_formatted":"Active","activation_limit":5,"instances_count":1,"expires_at":null,"product_name":"…","variant_name":"…"}],
  "files":[{"id":1,"name":"app.zip","extension":"zip","size_formatted":"854 KB","version":"1.0.0","product_name":"…","variant_name":"…","download_url":"/download/<uuid>?expires=…&signature=…"}],
  "links":[{"title":"Docs","url":"https://…"}],
  "subscriptions":[{"id":1,"status":"active","status_formatted":"Active","product_name":"…","renews_at":"…","ends_at":null}],
  "billing_url":"/billing?expires=…&signature=…" ,
  "invoice_url":"https://…"
}
```

`download_url`/`billing_url` are relative paths; the browser can navigate to them directly (the backend responds with a 302 or streams). **Page routes (Next.js)**: `/my-orders` (email sign-in + order list), `/my-orders/verify` (magic link landing page), `/my-orders/[identifier]` (order detail, supports `?signature=`).

## 8. Supplementary conventions (the implementation follows these)

**Permission matrix**

| Resource | viewer | admin | owner |
| --- | --- | --- | --- |
| Most lists/details (orders, customers, subscriptions, license-keys, products, webhooks, deliveries, stripe-events, stores, audit-logs, email-logs, imports) | Read | Read/write | Read/write |
| `GET /settings`, `GET /api-keys`, `GET /stripe/*` write operations, Webhook configuration, etc. | ✗ | ✓ | ✓ |
| `PUT /settings`, `POST/DELETE /api-keys`, `/users`, LS import (`POST /imports/lemonsqueezy*`) | ✗ | ✗ | ✓ |
| Own profile `PATCH /me`, `POST /me/password`, `DELETE /me/stripe` | ✓ | ✓ | ✓ |

**General**
- Validation failure `422` (`validation_failed`, with `fields`; settings keys are dot paths, e.g. `smtp.port`); created `201`; update/action `200`; delete `204`; permission/CSRF `403`; not signed in `401`; resource not found `404`; unique conflict/cannot delete `409`; Stripe upstream error `502` (`stripe_error`).
- Detail endpoints fetch by ID and are **not affected by the `test_mode` parameter** (switching test mode will not cause a 404).
- Clearing nullable fields: for strings, either `""` or `null` works (`PATCH /customers`, `stripe_*` in `PATCH /users`; nullable numbers in `PATCH /prices` use `null`).
- `POST /me/password`: when the account has no password yet (Stripe sign-in only), `current_password` may be empty, used to set the first password.
- All paginated lists return `{data, meta}`; the following return a **bare array**: `GET /imports/{id}/logs`, `GET /webhook-events`, `GET /stripe/catalog`.
- `StripeEvent.id` is the Stripe event ID **string** (`evt_…`), not a number.

**Response shapes**
- `POST /orders/{id}/refund` → the updated `Order` (same shape as a list item).
- `POST /license-keys` → the newly created `LicenseKey` detail (including `instances`).
- `POST /stripe/catalog/import` → `{"products":n,"variants":n,"prices":n,"skipped":n}`.
- `PUT /settings` → the same full structure as `GET /settings`.
- `POST /imports/*` → `ImportJob`. `ImportJob.options` never contains the LS API key in plaintext (only the last 4 characters are kept).
- `ImportJob.kind`: `lemonsqueezy_api` | `lemonsqueezy_files` (ZIP) | `stripe_mapping` | `stripe_auto_match`. Only one job of the same kind can run at a time (starting another returns 409).
- `progress`: `{stage, done, total, counts:{…}, warnings, errors?, dry_run?}`; besides the per-resource counts, `counts` may also include `invalid`, `failed`, `conflicts` (rows whose ID is taken by another local record) and `overwritten` (rows overwritten when `on_conflict=overwrite`). `options.on_conflict` echoes the strategy in effect. `done/total` is progress for the **current stage**; `total=0` means unknown. `stage` values: for LS imports, the resource name (`stores customers products variants prices files orders order_items subscriptions subscription_items subscription_invoices license_keys license_key_instances`); for mapping jobs, `customers|products|subscriptions`; for auto-match, `customers products prices subscriptions`.
- File upload: if the form's `name` has no extension, the uploaded file's extension is appended automatically.
