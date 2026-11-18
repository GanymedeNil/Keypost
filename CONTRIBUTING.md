# Contributing

Thanks for helping with Keypost. Keypost handles payments and license entitlements, so changes are reviewed and tested before they reach `main`.

## Issues

Use the issue forms: **Bug report**, **Lemon Squeezy compatibility difference** (include both responses side by side) and **Feature request**. Never paste secrets, license keys or customer data into an issue. Mask Stripe keys, Lemon Squeezy API keys, `APP_SECRET`, signed URLs and customer e-mail addresses first.

## Pull requests only

Nobody pushes to `main` directly, including the maintainers. Every change goes through a pull request:

```bash
git switch -c my-change
# …edit, commit…
git push -u origin my-change     # then open a pull request against main
```

A pull request can be merged once all four CI checks pass (`.github/workflows/ci.yml`):

| Check | What it runs |
| --- | --- |
| **Backend (SQLite)** | `gofmt`, `go vet`, `go test -race` on an in-memory SQLite database |
| **Backend (PostgreSQL 18)** | the same test suite against a real PostgreSQL 18 |
| **Frontend** | `pnpm lint`, `pnpm typecheck`, `pnpm build` |
| **Version** | `scripts/check-version.sh`: `VERSION`, `frontend/package.json` and the top `CHANGELOG.md` entry agree |

CI also runs on every push to `main`, that is, on every merge.

## Run the checks locally

```bash
make fmt          # gofmt -w
make lint         # go vet + eslint + tsc
make test         # backend tests, SQLite in memory, with -race
make test-pg      # backend tests on PostgreSQL 18 (needs Docker)
scripts/check-version.sh
cd frontend && pnpm build
```

## Guidelines

- Keep Lemon Squeezy response shapes, IDs and error messages stable. Existing clients and importers depend on them.
- Change `docs/ADMIN_API.md` first when you change an admin or storefront endpoint, and update `docs/API_COMPATIBILITY.md` when compatibility changes.
- Add or update tests for behavior changes. Money paths (checkout, refunds, subscriptions, webhooks) need a test.
- Docs and screenshots are in English.
- User-visible changes get a line in `CHANGELOG.md` under an "Unreleased" heading. Release steps are at the top of that file.
- By contributing you agree that your contribution is licensed under the [MIT License](LICENSE). Read the [Disclaimer](README.md#disclaimer): Keypost is provided as is.
