#!/usr/bin/env bash
# Fails when the version numbers drift apart: the backend VERSION file is the
# source of truth; frontend/package.json and the top CHANGELOG entry must match.
set -euo pipefail
cd "$(dirname "$0")/.."

backend=$(tr -d '[:space:]' < backend/internal/version/VERSION)
frontend=$(node -p "require('./frontend/package.json').version")
changelog=$(sed -nE 's/^## \[([0-9][^]]*)\].*/\1/p' CHANGELOG.md | head -1)

status=0
[ "$backend" = "$frontend" ] || { echo "version mismatch: backend/internal/version/VERSION=$backend, frontend/package.json=$frontend"; status=1; }
[ "$backend" = "$changelog" ] || { echo "version mismatch: backend/internal/version/VERSION=$backend, top CHANGELOG.md entry=$changelog"; status=1; }
[ $status -eq 0 ] && echo "version $backend is consistent"
exit $status
