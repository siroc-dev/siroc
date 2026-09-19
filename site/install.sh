#!/bin/bash
# Siroc installer — downloads the latest package from the update channel (Cloudflare R2).
# Usage: curl -fsSL https://<site>/install.sh | sudo bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root: curl -fsSL <url>/install.sh | sudo bash" >&2
  exit 1
fi

CHANNEL="${SIROC_UPDATE_URL:-__SIROC_RELEASE_BASE__}"
CHANNEL="${CHANNEL%/}"
if [ -z "$CHANNEL" ] || [ "$CHANNEL" = "__SIROC_RELEASE_BASE__" ]; then
  echo "Set SIROC_UPDATE_URL to your public R2 / update channel URL (the folder that has latest.json)." >&2
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

echo "Fetching $CHANNEL/latest.json"
META="$WORKDIR/latest.json"
if ! curl -fsSL "$CHANNEL/latest.json" -o "$META"; then
  echo "could not download latest.json from $CHANNEL" >&2
  exit 1
fi

python3 - "$META" "$WORKDIR" <<'PY'
import json, sys, pathlib
meta = json.loads(pathlib.Path(sys.argv[1]).read_text())
url = (meta.get("url") or "").strip()
if not url:
    raise SystemExit("latest.json is missing url")
pathlib.Path(sys.argv[2], "pkg.url").write_text(url)
pathlib.Path(sys.argv[2], "pkg.ver").write_text(str(meta.get("version") or ""))
pathlib.Path(sys.argv[2], "pkg.sha").write_text(str(meta.get("sha256") or ""))
PY

URL="$(cat "$WORKDIR/pkg.url")"
VER="$(cat "$WORKDIR/pkg.ver")"
SHA="$(cat "$WORKDIR/pkg.sha")"
TAR="$WORKDIR/siroc.tar.gz"

echo "Downloading Siroc ${VER:-package}"
curl -fL --progress-bar "$URL" -o "$TAR"

if [ -n "$SHA" ]; then
  echo "$SHA  $TAR" | sha256sum -c -
fi

echo "Extracting..."
tar -xzf "$TAR" -C "$WORKDIR"
if [ -x "$WORKDIR/siroc/install.sh" ]; then
  exec "$WORKDIR/siroc/install.sh"
fi
echo "package layout unexpected (wanted siroc/install.sh)" >&2
exit 1
