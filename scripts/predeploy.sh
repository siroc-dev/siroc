#!/bin/bash
# Run checks before publishing a site or panel release.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "→ go test"
go test ./internal/api ./internal/validate ./internal/admincli ./internal/setup ./internal/hosting ./internal/users ./internal/software ./internal/store ./internal/weblog

echo "→ web tests"
(cd "$ROOT/web" && npm test)

echo "→ web typecheck"
(cd "$ROOT/web" && npx tsc --noEmit)

echo "predeploy ok"
