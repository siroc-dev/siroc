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

ROOT="${1:-/siroc}"
DATA_DIR="${SIROC_DATA_DIR:-/siroc/data}"
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

mkdir -p "$ROOT/bin" "$ROOT/web" "$DATA_DIR" /siroc/log /siroc/cache /siroc/db /siroc/etc /var/run/siroc /usr/local/bin
chown siroc:siroc "$DATA_DIR"
chmod 750 "$DATA_DIR"

install -m 755 "$SRC/bin/siroc-agent" "$ROOT/bin/siroc-agent"
install -m 755 "$SRC/bin/siroc-panel" "$ROOT/bin/siroc-panel"
ln -sfn "$ROOT/bin/siroc-agent" /usr/local/bin/siroc-agent
ln -sfn "$ROOT/bin/siroc-panel" /usr/local/bin/siroc-panel
if [ -f "$SRC/bin/siroc" ]; then
  install -m 755 "$SRC/bin/siroc" "$ROOT/bin/siroc"
elif [ -f "$SCRIPT_DIR/siroc" ]; then
  install -m 755 "$SCRIPT_DIR/siroc" "$ROOT/bin/siroc"
elif [ -f "$SRC/scripts/siroc" ]; then
  install -m 755 "$SRC/scripts/siroc" "$ROOT/bin/siroc"
fi
if [ -f "$ROOT/bin/siroc" ]; then
  ln -sfn "$ROOT/bin/siroc" /usr/local/bin/siroc
fi
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
Environment=SIROC_UPDATE_URL=https://get.siroc.dev
RuntimeDirectory=siroc
RuntimeDirectoryMode=0750
ExecStart=/siroc/bin/siroc-agent
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
Environment=SIROC_UPDATE_URL=https://get.siroc.dev
ExecStart=/siroc/bin/siroc-panel
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

if [ "$TLS" != "0" ]; then
  echo "Requesting a Let's Encrypt certificate for the public IP..."
  if ! SIROC_DATA_DIR="$DATA_DIR" SIROC_TLS="$TLS" "$ROOT/bin/siroc-panel" issue-cert; then
    echo "Public certificate was not issued. The panel will use its own certificate until you issue one from Settings."
  fi
fi

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
private_ip() {
  local ip
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  printf '%s' "${ip:-127.0.0.1}"
}

valid_ipv4() {
  local ip="$1" octet
  [[ "$ip" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || return 1
  for octet in "${BASH_REMATCH[@]:1}"; do
    if [ "$octet" -gt 255 ]; then
      return 1
    fi
  done
}

lookup_public_ip() {
  local ip
  if ! command -v curl >/dev/null 2>&1; then
    return 0
  fi
  ip="$(curl -4 -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
  ip="${ip//$'\r'/}"
  ip="${ip//$'\n'/}"
  ip="${ip// /}"
  if valid_ipv4 "$ip"; then
    printf '%s' "$ip"
  fi
}

host_url() {
  local host="$1"
  if { [ "$TLS" = "0" ] && [ "$PORT" = "80" ]; } || { [ "$TLS" != "0" ] && [ "$PORT" = "443" ]; }; then
    printf '%s://%s' "$SCHEME" "$host"
  else
    printf '%s://%s:%s' "$SCHEME" "$host" "$PORT"
  fi
}

print_addresses() {
  local priv pub
  priv="$(private_ip)"
  pub="$(lookup_public_ip)"
  if [ -z "$pub" ]; then
    echo "  Private: $(host_url "$priv")"
    echo "  Public:  could not detect a public IP"
  elif [ "$pub" = "$priv" ]; then
    echo "  $(host_url "$priv")"
  else
    echo "  Private: $(host_url "$priv")"
    echo "  Public:  $(host_url "$pub")"
  fi
}

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
if [ -n "$TOKEN" ]; then
  for _ in $(seq 1 30); do
    if [ -s "$DATA_DIR/setup.url" ]; then
      break
    fi
    sleep 0.5
  done
fi

echo
echo "============================================================"
if [ -n "$TOKEN" ]; then
  echo "  Siroc is ready. Create the first admin here:"
  echo
  if [ -s "$DATA_DIR/setup.url" ]; then
    sed 's/^/  /' "$DATA_DIR/setup.url"
  else
    echo "  Private: $(host_url "$(private_ip)")/setup?token=${TOKEN}"
    echo "  Public:  could not detect a public IP"
  fi
  echo
  echo "  This one-time link is token-protected."
  echo "  Saved at $DATA_DIR/setup.url"
  echo "============================================================"
elif systemctl is-active --quiet siroc-panel.service; then
  echo "  Siroc is running. Open the panel and sign in."
  echo
  print_addresses
  echo "============================================================"
else
  echo "  Panel failed to start. Check: journalctl -u siroc-panel -u siroc-agent -e" >&2
  echo "============================================================"
  exit 1
fi
