#!/bin/bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -x "$SCRIPT_DIR/bin/siroc-panel" ]; then
  SRC="$SCRIPT_DIR"
elif [ -x "$SCRIPT_DIR/../bin/siroc-panel" ]; then
  SRC="$(cd "$SCRIPT_DIR/.." && pwd)"
else
  echo "bin/siroc-panel not found next to this script. Extract the package or build first (scripts/package.sh)." >&2
  exit 1
fi

ROOT="${1:-/opt/siroc}"
DATA_DIR="${SIROC_DATA_DIR:-/var/lib/siroc}"
LISTEN="${SIROC_LISTEN:-:8443}"
TLS="${SIROC_TLS:-1}"

need() {
  if [ ! -x "$SRC/bin/$1" ]; then
    echo "missing $SRC/bin/$1" >&2
    exit 1
  fi
}
need siroc-agent
need siroc-panel
if [ ! -f "$SRC/web/dist/index.html" ]; then
  echo "missing $SRC/web/dist (build the web UI first)" >&2
  exit 1
fi

echo "Installing Siroc from $SRC"

if ! getent group siroc >/dev/null; then
  groupadd --system siroc
fi
if ! id siroc >/dev/null 2>&1; then
  useradd --system -g siroc -d "$DATA_DIR" -s /usr/sbin/nologin siroc
fi

mkdir -p "$ROOT/bin" "$ROOT/web" "$DATA_DIR" /var/run/siroc /usr/local/bin
chown siroc:siroc "$DATA_DIR"
chmod 750 "$DATA_DIR"

install -m 755 "$SRC/bin/siroc-agent" /usr/local/bin/siroc-agent
install -m 755 "$SRC/bin/siroc-panel" /usr/local/bin/siroc-panel
if [ -f "$SRC/bin/siroc" ]; then
  install -m 755 "$SRC/bin/siroc" /usr/local/bin/siroc
elif [ -f "$SCRIPT_DIR/siroc" ]; then
  install -m 755 "$SCRIPT_DIR/siroc" /usr/local/bin/siroc
elif [ -f "$SRC/scripts/siroc" ]; then
  install -m 755 "$SRC/scripts/siroc" /usr/local/bin/siroc
fi
cp -a "$SRC/bin/." "$ROOT/bin/"
rm -rf "$ROOT/web/dist"
cp -a "$SRC/web/dist" "$ROOT/web/dist"
chmod -R a+rX "$ROOT/web/dist"
if [ -f "$SRC/VERSION" ]; then
  install -m 644 "$SRC/VERSION" "$ROOT/VERSION"
  install -m 644 "$SRC/VERSION" "$DATA_DIR/version"
fi

if [ -d "$SRC/lsp" ]; then
  rm -rf "$ROOT/lsp"
  cp -a "$SRC/lsp" "$ROOT/lsp"
  chmod -R a+rX "$ROOT/lsp" || true
fi

cat >/etc/systemd/system/siroc-agent.service <<'EOF'
[Unit]
Description=Siroc privileged agent
After=network.target

[Service]
Type=simple
Environment=HOME=/root
RuntimeDirectory=siroc
RuntimeDirectoryMode=0750
RuntimeDirectoryGroup=siroc
ExecStart=/usr/local/bin/siroc-agent
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/siroc-panel.service <<EOF
[Unit]
Description=Siroc web panel
After=siroc-agent.service
Requires=siroc-agent.service

[Service]
Type=simple
User=siroc
Group=siroc
Environment=SIROC_ROOT=$ROOT
Environment=SIROC_WEB_DIR=$ROOT/web/dist
Environment=SIROC_DATA_DIR=$DATA_DIR
Environment=SIROC_LISTEN=$LISTEN
Environment=SIROC_TLS=$TLS
ExecStart=/usr/local/bin/siroc-panel
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now siroc-agent.service
sleep 1
systemctl enable --now siroc-panel.service

SCHEME=https
if [ "$TLS" = "0" ]; then
  SCHEME=http
fi
PORT="${LISTEN##*:}"
if [ -z "$PORT" ] || [ "$PORT" = "$LISTEN" ]; then
  PORT=8443
fi
IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
IP="${IP:-127.0.0.1}"

TOKEN=""
for _ in $(seq 1 20); do
  if [ -s "$DATA_DIR/setup.token" ]; then
    TOKEN="$(tr -d '[:space:]' <"$DATA_DIR/setup.token")"
    break
  fi
  if [ -f "$DATA_DIR/panel.db" ] && [ ! -f "$DATA_DIR/setup.token" ]; then
    break
  fi
  sleep 0.5
done

echo
echo "============================================================"
if [ -n "$TOKEN" ]; then
  URL="${SCHEME}://${IP}:${PORT}/setup?token=${TOKEN}"
  echo "  Siroc is ready. Create the first admin here:"
  echo
  echo "    $URL"
  echo
  echo "  This one-time link is token-protected."
  echo "  Saved at $DATA_DIR/setup.url"
  echo "============================================================"
elif systemctl is-active --quiet siroc-panel.service; then
  echo "  Siroc is running: ${SCHEME}://${IP}:${PORT}"
  echo "  Admin already exists — open the panel and sign in."
  echo "============================================================"
else
  echo "  Panel failed to start. Check: journalctl -u siroc-panel -u siroc-agent -e" >&2
  echo "============================================================"
  exit 1
fi
