# Changelog

All notable changes to Keypost are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/). Versions below 1.0.0 may still change behavior between minor releases.

**Cutting a release:** bump `backend/internal/version/VERSION` and `frontend/package.json` to the same number, add the entry here, merge through a pull request, then tag the merge commit `vX.Y.Z`. `scripts/check-version.sh` (run in CI) keeps the three in step.

## [0.1.0] - 2026-11-18

First public release.

### Added

- Lemon Squeezy compatible License API (`/v1/licenses/*`), JSON:API (`/v1/*`) and outgoing webhooks (same events, payloads and HMAC-SHA256 signature).
- Stripe fulfillment: Checkout, renewals, failures, cancellations and refunds create and update orders, subscriptions and license keys; signed file downloads; receipt e-mails.
- Lemon Squeezy migration: import from the API or an offline archive, dry run, ID-conflict guard, LS-to-Stripe mapping, auto-match and a mapping report.
- Admin back office (Next.js, shadcn/ui; English and Chinese; light and dark): orders, customers, subscriptions, license keys, products and files, webhook log, Stripe event replay, API keys, team and audit log.
- Customer pages ("My orders") with magic-link sign-in and the Stripe billing portal.
- Mandatory two-factor authentication (TOTP) for every member, "Sign in with Stripe", and the `dfctl` operator CLI (`dfctl version` prints the version).
- PostgreSQL 18 and SQLite support, local or S3 file storage, Docker Compose deployment.

See the [Disclaimer](README.md#disclaimer) before using Keypost with real money.
