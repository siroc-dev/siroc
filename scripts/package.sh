#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist/siroc-linux-amd64.tar.gz}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

cd "$ROOT"

echo "Building web UI..."
(cd "$ROOT/web" && npm install && npm run build)

VER="$(tr -d '[:space:]' <"$ROOT/VERSION" 2>/dev/null || echo 0.0.0)"
echo "Building Linux binaries (v$VER)..."
mkdir -p "$ROOT/bin"
LDFLAGS="-s -w -X github.com/siroc-dev/siroc/internal/version.Version=${VER}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$ROOT/bin/siroc-agent" ./cmd/agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$ROOT/bin/siroc-panel" ./cmd/panel

PKG="$STAGE/siroc"
mkdir -p "$PKG/bin" "$PKG/web"
cp -a "$ROOT/bin/siroc-agent" "$ROOT/bin/siroc-panel" "$PKG/bin/"
cp -a "$ROOT/web/dist" "$PKG/web/dist"
echo "$VER" >"$PKG/VERSION"
install -m 755 "$ROOT/scripts/install.sh" "$PKG/install.sh"
if [ -d "$ROOT/lsp/node_modules" ]; then
  mkdir -p "$PKG/lsp"
  cp -a "$ROOT/lsp/package.json" "$PKG/lsp/" 2>/dev/null || true
  cp -a "$ROOT/lsp/node_modules" "$PKG/lsp/node_modules"
fi

mkdir -p "$(dirname "$OUT")"
tar -C "$STAGE" -czf "$OUT" siroc
echo "Wrote $OUT"
echo "On the server: tar -xzf $(basename "$OUT") && cd siroc && sudo ./install.sh"
