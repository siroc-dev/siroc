#!/bin/bash
set -euo pipefail

ROOT="${SIROC_ROOT:-/opt/siroc}"
cd "$ROOT"

if ! getent group siroc >/dev/null; then
  groupadd --system siroc
fi
if ! id siroc >/dev/null 2>&1; then
  useradd --system -g siroc -d /var/lib/siroc -s /usr/sbin/nologin siroc
fi

mkdir -p /var/lib/siroc /var/run/siroc /home
chown siroc:siroc /var/lib/siroc
chmod 750 /var/lib/siroc

export PATH="/usr/local/go/bin:$PATH"
export GOPATH="/root/go"
export GOMODCACHE="/root/go/pkg/mod"
export GOCACHE="/root/.cache/go-build"
export HOME="/root"
export npm_config_cache="/root/.npm"

VER="$(tr -d '[:space:]' </opt/siroc/VERSION 2>/dev/null || echo 0.0.0)"
echo "Building Go binaries (v$VER)..."
LDFLAGS="-s -w -X github.com/siroc-dev/siroc/internal/version.Version=${VER}"
go build -ldflags="$LDFLAGS" -o /usr/local/bin/siroc-agent ./cmd/agent
go build -ldflags="$LDFLAGS" -o /usr/local/bin/siroc-panel ./cmd/panel
install -m 755 "$ROOT/scripts/siroc" /usr/local/bin/siroc
echo "$VER" >/opt/siroc/VERSION
echo "$VER" >/var/lib/siroc/version

echo "Building web UI..."
cd "$ROOT/web"
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=4096}"
if [ ! -d node_modules ]; then
  npm install
fi
npm run build
chmod -R a+rX "$ROOT/web/dist"

echo "Installing language servers..."
cd "$ROOT/lsp"
npm install --omit=dev
chmod -R a+rX "$ROOT/lsp" || true

systemctl daemon-reload
systemctl enable siroc-agent.service siroc-panel.service
systemctl restart siroc-agent.service
sleep 1
systemctl restart siroc-panel.service

TOKEN=""
for _ in $(seq 1 20); do
  if [ -s /var/lib/siroc/setup.token ]; then
    TOKEN="$(tr -d '[:space:]' </var/lib/siroc/setup.token)"
    break
  fi
  if [ -f /var/lib/siroc/panel.db ] && [ ! -f /var/lib/siroc/setup.token ]; then
    break
  fi
  sleep 0.5
done

echo "Siroc is up on http://localhost:8443"
if [ -n "$TOKEN" ]; then
  echo "Create the first admin (token-protected):"
  echo "  http://localhost:8443/setup?token=${TOKEN}"
fi
