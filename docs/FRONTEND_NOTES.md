# Frontend notes

Next.js 16 (App Router, React 19, TypeScript strict, Tailwind v4, **shadcn/ui** (new-york, neutral), SWR, lucide icons) in `frontend/`.
Admin back office under `/admin/*`, customer pages under `/my-orders*`, `/` redirects to `/my-orders`.

## Run it

```bash
cd frontend
pnpm install
pnpm mock                      # mock backend on :8080 (PORT=… to change)
BACKEND_URL=http://localhost:8080 pnpm dev      # http://localhost:3000
pnpm typecheck && pnpm lint && pnpm build       # build also copies static assets into .next/standalone
pnpm start                     # node .next/standalone/server.js (PORT / HOSTNAME env respected)
```

Demo logins on the mock: `owner@example.com`, `admin@example.com`, `viewer@example.com` (password `password1234`).
Mock switches: `MOCK_SETUP=1` (first-run setup, token `setup-token-123`), `MOCK_STORES=1`, `MOCK_STRIPE_LOGIN=0`,
`MOCK_LATENCY=0`, `PUBLIC_URL`. Any magic-link token works except `invalid` / `expired`; LS import api_key `fail` fails the job.

`BACKEND_URL` is evaluated by `next.config.ts` rewrites at **build time** (baked into `routes-manifest.json`), so in Docker pass it
as `--build-arg BACKEND_URL=…`. The `proxy.ts` gate only matches `/admin` and `/admin/*`, never `/admin-api/*`, so multipart uploads are
streamed through the rewrite without being buffered (verified with a 300 MB upload); `experimental.proxyClientMaxBodySize` is still
set to `2gb` as a safety net.

Next 16 renamed `middleware.ts` to `proxy.ts`; that file implements the "no `df_admin` cookie → `/admin/login?next=…`" gate.
TypeScript is pinned to 6.0.x because `typescript-eslint` (used by `eslint-config-next`) does not support TS 7 yet.

## Layout

| Area | Files |
| --- | --- |
| Config | `next.config.ts` (rewrites, standalone), `proxy.ts`, `Dockerfile`, `.env.example`, `scripts/postbuild.mjs` |
| API layer | `lib/api.ts` (client, `ApiError`, upload w/ progress), `lib/types.ts` (mirrors ADMIN_API.md), `lib/use-api.ts` (`useApi`, `useList`), `lib/use-action.ts`, `lib/swr.ts` (`invalidate`), `lib/url-state.ts` |
| i18n | `lib/i18n/{index,provider,define}.ts(x)`, `lib/i18n/messages/<area>.ts` (en + zh side by side, key parity enforced by TS), language from cookie `df-lang` → `Accept-Language` |
| shadcn/ui | `components/ui/*` are the stock shadcn components (button, card, input, dialog, sheet, dropdown-menu, tabs, switch, checkbox, table, badge, progress, skeleton, tooltip…), added with `pnpm dlx shadcn@latest add <name>`; config in `components.json`. Don't edit them for app needs |
| UI kit | `components/kit/*` is the app-level API pages import from `@/components/kit`: Button (variants primary / secondary / ghost / danger), Field/Input/Select, Switch, Badge/StatusBadge, Card (Header/Body/Footer), DataTable, Tabs, Modal/Drawer, ActionMenu, Toast (Sonner), ConfirmProvider, JsonViewer, CopyButton, states, Progress… Each wraps the shadcn component, so restyling happens in one place |
| Theme | `app/globals.css`: shadcn CSS variables (`--background`, `--foreground`, `--primary`, `--muted`, `--border`, `--sidebar-*`, `--radius`…) for light and dark, plus `success / warning / danger / info` extras (`bg-success-soft text-success-text` …). The logo, the primary button and the e-mails use the same near-black ink. Use the shadcn token classes (`bg-card`, `text-muted-foreground`, `border-border`) |
| Admin shell | `app/admin/(shell)/layout.tsx`, `components/admin/{sidebar,topbar,admin-context,nav}` (store switcher, global test mode, theme, language, user menu) |
| Dashboard | `app/admin/(shell)/page.tsx`, `components/admin/dashboard/*` (hand-rolled SVG chart, KPIs, health card) |
| Orders / Customers | `orders/**`, `customers/**` + `components/admin/{orders,customers}` |
| Subscriptions / License keys | `subscriptions/**`, `license-keys/**` + `components/admin/{subscriptions,licenses}` |
| Products | `products/**` + `components/admin/products` (variants, prices, links, files upload, Stripe mapping) |
| Webhooks | `webhooks/` (endpoints), `webhooks/deliveries` + `components/admin/webhooks` |
| Stripe | `stripe/` (`?tab=status|events|catalog`) + `components/admin/stripe` |
| Migration | `imports/`, `imports/[id]` + `components/admin/imports` |
| Settings | `settings/` (General/Stores), `stripe`, `email`, `license`, `api-keys`, `team`, `audit-log`, `email-log`, `profile` + `components/admin/{settings,team}` |
| Customer pages | `app/my-orders/**` + `components/storefront`, `lib/storefront.ts` |
| Mock backend | `mock/server.mjs` (+ `fixtures.mjs`, `lib.mjs`), no dependencies |

Conventions: all admin lists use `useList` (URL-synced `page/q/sort/filters`, auto `test_mode` + `store_id`); detail pages use `useApi`
(auto `test_mode`); mutations go through `useAction().run(...)` (toasts + field errors); viewers are read-only through `useCan()`.

## Contract gaps / ambiguities (please confirm or add to ADMIN_API.md)

Roles and access
1. Role paragraph says admin cannot write API keys/users/secrets, but the `api-keys` heading says "owner/admin" and `PUT /settings` is "owner". UI: all Settings writes and API-key create/revoke are owner-only (admins read), Team is owner-only. Tell me if admins should write non-secret settings or API keys.
2. `GET /stores` is needed by every role for the store switcher, but `stores` is "CRUD (owner/admin)". UI tolerates a 403 (no switcher). Suggest allowing GET for viewers or returning `stores` in `GET /me`.
3. `GET /settings` is 403 for viewers; viewers only get Profile / Audit log / Email log tabs. Whether viewers may `PATCH /me` is unclear (UI allows it).
4. There is no endpoint to set a first password for a Stripe-only user (`has_password=false`); the Profile page shows an info note instead of a password form.

Conventions
5. HTTP status for `validation_failed` is not specified (UI keys off `error.code`; mock uses 422). Create endpoints: 200 vs 201 not pinned (UI accepts both).
6. Detail endpoints receive `test_mode` from the global switch. They should not 404 when the record's mode differs from the query param (or the UI will show "not found" after toggling test mode on a detail page).
7. Clearing nullable fields: `PATCH /customers` and `PATCH /users` `stripe_*` ids: `null` vs `""` (UI sends `null` for customers, `""` for users, as the mock accepts both).
8. Validation `fields` keys for settings are dotted (`smtp.port`, `stripe.login_mode`); forms read them that way.
9. Lists: all list endpoints must return `{data,meta}` (UI breaks on a bare array, incl. `GET /imports` and `GET /stores`). `GET /imports/{id}/logs` may be a bare array (UI accepts either).

Shapes not pinned
10. Responses of `POST /orders/{id}/refund` (UI refetches the order), `POST /license-keys` (UI assumes a bare `LicenseKey` to redirect), `POST /stripe/catalog/import`, `POST /imports/stripe-mapping` (assumed `ImportJob`), `PUT /settings` (assumed full `Settings`).
11. `Customer`, `Product`, `Variant` have no currency; UI uses the store's currency. `Subscription.pause` shape, `Order.currency_rate`/`tax_rate` (number vs string) and `PriceTier.last_unit` (number or `"inf"`) are loose.
12. `StripeEvent.id` is numeric in the contract; `StripeEventDetail.payload` may be an object or a JSON string (UI handles both), `Delivery.payload` is a string.
13. Import `kind` values and `progress.stage` names are not in the contract (UI shows mock names with a humanised fallback); `progress.done/total` is assumed per stage. `ImportJob.options` may hold `api_key`: UI masks option keys named `api_key`/`apikey`/`token`/`secret` — please never return the LS key.
14. `GET /webhook-events` returns bare names; UI groups by the prefix before the first `_`. Webhook `secret` is optional on create in the UI (6–40 chars when given).
15. Stripe event `type` filter options come from `required_events` (no list endpoint); audit-log `resource_type` and email-log `status` are free-form, UI offers a static list plus values seen.
16. Pricing: `link-stripe` takes a single `stripe_price_id` (UI offers one; multi-price variants have no per-price link). PWYW prices send `unit_price = min_price` (mock convention). The UI only creates `scheme=standard`; tiered/package prices are read-only (their tiers are preserved on PATCH). Deleting a variant/price referenced by orders is assumed to be a 409.
17. Refunds: UI only treats an order as Stripe-refundable when `source=stripe` and `stripe_payment_intent` is set, otherwise it shows a warning before calling the API.
18. File upload `name` is sent as the file name without extension.
19. Storefront: any 400 from `POST /auth/verify` is treated as an invalid/expired link. Storefront POSTs send `X-Requested-With: df-admin` as the contract says.

## Known limitations

- `BACKEND_URL` is a build-time setting (Next.js rewrites); change it by rebuilding.
- The chart is hand-rolled SVG (hover tooltip; no zoom/brush). The dashboard polls every 60 s.
- Language/theme switchers persist per browser (cookie + localStorage); the admin has no per-user language preference.
- Admin role checks are UI-level conveniences; the API is the source of truth (it returns 403).
- Large uploads depend on the backend `MAX_UPLOAD_MB`; the browser shows per-file progress but cannot resume.
- Dev only: the Next.js dev HMR websocket is not proxied through `/admin-api` and needs direct access to the dev server port.
