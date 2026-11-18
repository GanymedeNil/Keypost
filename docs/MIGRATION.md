# Migrating from Lemon Squeezy

> Timeline (official): 2026-11-16 new signups stop; 2026-12-02 migration tool goes live; start migrating by mid-January 2027 at the latest; **2027-03-31 transaction processing stops (including subscription renewals), with no extension**.
> LS's official migration tool only carries over customers, products, and card subscriptions; **you must carry over order history, license keys, files, and download access yourself**—which is exactly what this service's importer does.

## 0. One principle first: keep the original IDs

On import, **all Lemon Squeezy IDs are kept** (`license_keys.id`, `orders.id`, `customers.id`, order `identifier`/`order_number`, license instance `identifier` = `instance_id` …),
so any ID, key, or instance ID stored in old clients and your own business systems stays valid on the new service. Therefore:

- **Import into an empty database (or one that contains only imported data).** New auto-increment IDs continue after the maximum ID (PostgreSQL sequences are reset automatically after import). LS IDs are large numbers shared across the whole platform and usually do not collide with the new service's small auto-increment IDs, but we still recommend importing first and taking payments afterward.
- **ID conflict guard.** Before writing each batch, the importer checks whether any of these IDs is already taken in this database by a **different record** (for example an order created by Stripe checkout, or a hand-created product). On a conflict it **stops the import**—it neither overwrites nor lets later variants/order items/license keys quietly attach to that local record.
  - How "the same record" is determined: `created_at` matches the source (always true on re-import; local edits do not change `created_at`), or the natural key matches—customer (store + email), order (`identifier`), license key (key string), instance and file (`identifier`), store (`slug`, so a fresh install's placeholder store is taken over by the LS store with the same slug). Other tables compare `created_at` only. Source objects must therefore carry `created_at` (both the LS API and official exports do; add it to hand-written archives). `orders.csv` is merged by email and order number and is not part of this check.
  - The log lists each conflict: table, ID, and a summary of the local and source data. Earlier batches already written are valid LS data and are kept; once the conflicts are resolved, rerun directly.
  - **A dry run performs the same check**, lists **all** conflicts, then ends in failure (a real import stops at the first conflicting batch), so the pre-launch dry run is the conflict check.
  - Fixes: (1) import into an empty database (recommended); (2) if the conflicting local data is disposable test data, turn on "Overwrite conflicting ids" in the import form (API parameter `on_conflict: "overwrite"`; the archive upload form field has the same name). The conflicting data is replaced and each replacement is recorded in the warning log.
- Re-importing is safe: idempotent upsert by primary key; local Stripe links (`stripe_*`), activated instances, and download counts are not overwritten; rows whose local update time is newer are not rolled back by older LS data.

## 1. Preparation

1. Deploy this service (see README), enter the keys under *Settings → Stripe & login* in the admin, and configure the webhook. **Do not switch live checkout over yet.**
2. In the LS dashboard, create an **API key** (read-only use is enough; create one each in Live and Test mode if you need to import test data).
3. Want to try it first? Spin up a temporary instance with `dfctl demo-data` or `docker compose`, and choose **Dry run** when importing.

## 2. Importing Lemon Squeezy data

On the admin **Migration** page there are three entry points:

### A. Pull directly from the LS API (recommended)
Enter the API key → choose resources (all by default) → check "Download files" → we recommend a **dry run first**: it only fetches, validates, and counts; nothing is written or downloaded.
The importer processes in dependency order: `stores → customers → products → variants → prices → files → orders → order-items → subscriptions → subscription-items → subscription-invoices → license-keys → license-key-instances`.

- Respects LS rate limits (300 requests/minute) and automatically backs off and retries on 429/5xx; `links.next` is only followed within your configured LS domain, so the API key is never sent to any other host.
- **Files**: LS's `download_url` is valid for only 1 hour, so files are **downloaded immediately** during import into this service's storage (local disk or S3), keeping `identifier`, version, sort order, and status. A failed download logs a warning; rerun to fill it in.
- The API key is **used in memory only**; the job record keeps just the last 4 characters (`****abcd`).

### B. Offline ZIP (the API key never leaves your computer)
```bash
LEMONSQUEEZY_API_KEY=xxxxx python3 scripts/export-lemonsqueezy.py ls-export.zip --files
```
Then choose "Upload archive (ZIP)" in the admin. Archive format (the importer accepts all of these):

| Content | Description |
| --- | --- |
| `<resource>.json` (may be several, e.g. `orders-1.json`, `orders-2.json`, or in subdirectories) | File names start with the resource name: `stores customers products variants prices files orders order-items subscriptions subscription-items subscription-invoices license-keys license-key-instances`. Content may be a JSON:API document (`{"data":[…]}`), an array of resource objects, or NDJSON |
| `files/<identifier>` | The file itself (`<id>.zip`, `<id>/filename` and `<id>-filename` are also recognized). With no file and "Download files" checked, `download_url` is tried |
| `orders.csv` | **Fallback**: the CSV from LS "Export orders" (see the LS help docs for columns). Used only when there is no `orders.json`; matches existing variants by name, and order items that cannot be matched log a warning |

Security limits: zip-slip paths are rejected, each entry ≤ 4 GiB, entry count ≤ 200,000, nothing is extracted to disk.

### C. Orders CSV only
Put just an `orders.csv` in the ZIP to get customers + orders + order items (no license keys/files—the CSV does not contain them).

The **job page** shows the stage, counts, and warnings live, with an incremental log, and you can cancel at any time. Common warnings:
- `PayPal subscription(s) were imported for history only` — LS officially does not support moving PayPal subscriptions elsewhere either; these customers need to resubscribe with a new payment method.
- `stored N bytes but Lemon Squeezy reported M` — file size differs from LS's record; please verify manually.
- Duplicate customer emails: merged into the smallest ID, and the customer IDs of orders/subscriptions/keys are remapped (recorded in the log).

Import **does not** trigger outgoing webhooks and **does not** send email.

## 3. Linking Stripe

LS's migration tool (Migration Hub, from 2026-12-02) turns each **variant into a Stripe product** and provides an "LS customer ID → Stripe customer ID" mapping report. This service needs these links so that Stripe webhooks can match historical data:

| Object | Link field | Purpose |
| --- | --- | --- |
| Customer | `customers.stripe_customer_id` | Customer portal; renewal/refund events are attributed to the existing customer |
| Variant / price | `variants.stripe_product_id`, `prices.stripe_price_id` | New orders are matched to a variant by Stripe price, so keys/files are issued |
| Subscription | `subscriptions.stripe_subscription_id` | Renewal, cancellation, and expiry events apply to the historical subscription and its license keys |

Three methods (can be combined):

1. **Upload a mapping CSV** (admin → Migration → Step 2, choose type: Customers / Products / Subscriptions). Headers are detected automatically: by keywords (`lemon`, `ls`, `customer`, `stripe`, `product`, `price`, `subscription`…) **and** by value shape (`cus_…` `prod_…` `price_…` `sub_…`); comma / semicolon / tab separators, BOM, and headerless files all work; customers can fall back to matching by email. Result stats: matched / unchanged / unknown_ls_id / invalid_rows / conflicts; the first 50 problems are written to the log.
2. **Auto-match** (Step 3): customers are looked up in Stripe by email (if several, the earliest created is taken, with a warning); products by metadata (`variant_id`/`ls_variant_id`) → name (`variant name`, `product - variant`, `product (variant)`, `product — variant`, case-insensitive) → price (amount + interval + currency); subscriptions are matched by "customer + price/product", then status is synced from Stripe immediately.
3. **Manual**: "Link to Stripe" on the variant page, "Link Stripe subscription" on the subscription page, or edit `stripe_customer_id` on the customer page.

Handle whatever is still unmatched item by item with **Download mapping report** (`GET /admin-api/imports/mapping-report.csv`): it lists customers/variants/prices/unexpired subscriptions missing a Stripe ID (PayPal subscriptions are marked "cannot migrate to Stripe").

> Note: auto-match does not distinguish test/live mode—do not mix LS test data with Stripe live customers (import Live and Test separately).

## 4. Cutover

1. **License API**: replace `https://api.lemonsqueezy.com` in your clients/servers with `https://<your-domain>`. Paths, parameters, responses, and error messages are unchanged; keys and instance IDs carry over.
2. **JSON:API and SDKs**: again change only the base URL, and replace the API key with one newly created under "API keys" in the admin (`dfk_live_…`).
3. **Webhook**: create an endpoint under *Webhooks* in the admin (you choose the events and the signing secret, 6–40 characters). The signing algorithm, headers, and payload match LS, so **no changes to signature verification code are needed**. Confirm with "Send test" and the delivery log.
4. **Checkout entry**: use the product's `buy_now_url`, or point your `POST /v1/checkouts` calls at the new domain (`checkout_data.custom`, `product_options.redirect_url`, `custom_price`, etc. carry over). LS checkout links/overlays do not redirect automatically—you must replace them yourself; change all old links everywhere.
5. **Customer entry**: `/my-orders` (email magic-link sign-in). Tell customers to update their old LS My Orders links; receipt emails already carry the new link.
6. **Stripe**: subscription migration goes through the LS migration tool (notify customers at least 14 days ahead); after that, renewal events for subscriptions in Stripe automatically update the subscriptions and license keys in this service.

## 5. Acceptance checklist

- [ ] Spot-check old customers' keys: `POST /v1/licenses/validate` (with `instance_id`) → `valid:true`, and `activation_usage` matches LS.
- [ ] New activation, limit exceeded, and deactivation behave the same as LS (`This license key has reached the activation limit.`).
- [ ] The order page (`/my-orders/<identifier>?signature=…`) shows the keys, file downloads (opening one yields the correct bytes), and links.
- [ ] The dry run shows no ID conflicts, and the real import's job page shows an "Id conflicts" count of 0.
- [ ] The admin mapping report is empty, or all remaining items are known (PayPal subscriptions, etc.).
- [ ] In the Stripe test environment: new purchase → order/key/receipt email/outgoing webhook; renewal; cancellation → key expires; refund → key disabled.
- [ ] The webhook signing secret is configured (Stripe page), and the Events tab on the Stripe page shows no `failed`.
- [ ] Storage is backed up (local `DATA_DIR/files` or S3 bucket) + database backup.

## 6. Rollback and safeguards

- LS remains usable until 2027-03-31; import is a read-only pull and never modifies LS data.
- Before the real cutover, you can repeat dry runs / rerun imports as often as needed; if something goes wrong, start over on an empty database.
- Keep LS's order export CSV and tax documents (inaccessible after 2027-03-31).
- Failed outgoing webhooks keep their delivery records and can be redelivered one by one in the admin; failed Stripe event processing can be retried with "Replay" on the Stripe page.
