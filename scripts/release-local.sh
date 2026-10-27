#!/bin/bash
# Build the Linux panel package and upload it to Cloudflare R2 from this machine.
# Same objects as .github/workflows/release.yml.
#
# Credentials (environment, or a gitignored .r2.env in the repo root):
#   CLOUDFLARE_ACCOUNT_ID
#   R2_ACCESS_KEY_ID
#   R2_SECRET_ACCESS_KEY
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SKIP_BUILD=0
SKIP_UPLOAD=0
SKIP_PRUNE=0
VER="${VERSION:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --skip-build) SKIP_BUILD=1 ;;
    --skip-upload) SKIP_UPLOAD=1 ;;
    --skip-prune) SKIP_PRUNE=1 ;;
    --version) VER="${2:-}"; shift ;;
    *) echo "unknown flag: $1" >&2; exit 1 ;;
  esac
  shift
done

if [ -f "$ROOT/.r2.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/.r2.env"
  set +a
fi

if [ -z "$VER" ]; then
  VER="$(tr -d '[:space:]' <"$ROOT/VERSION")"
fi
VER="${VER#v}"
BASE="${SIROC_RELEASE_PUBLIC_BASE:-https://get.siroc.dev}"
BASE="${BASE%/}"
PKG="$ROOT/dist/siroc-linux-amd64.tar.gz"
META="$ROOT/dist/latest.json"

if [ "$SKIP_BUILD" -eq 0 ]; then
  echo "=== Build $VER ==="
  bash "$ROOT/scripts/package.sh" "$PKG"
fi
if [ ! -f "$PKG" ]; then
  echo "missing $PKG" >&2
  exit 1
fi

SHA="$(sha256sum "$PKG" | awk '{print $1}')"
URL="${BASE}/${VER}/siroc-linux-amd64.tar.gz"
python3 -c 'import json,sys; print(json.dumps({"version":sys.argv[1],"url":sys.argv[2],"sha256":sys.argv[3]}, indent=2))' "$VER" "$URL" "$SHA" > "$META"
echo "Published metadata:"
cat "$META"

if [ "$SKIP_UPLOAD" -eq 1 ]; then
  echo "Skip upload. Package is at $PKG"
  exit 0
fi

if [ -z "${R2_ACCESS_KEY_ID:-}" ] || [ -z "${R2_SECRET_ACCESS_KEY:-}" ]; then
  echo "Set R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY (or put them in .r2.env)." >&2
  exit 1
fi
if [ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ]; then
  echo "Set CLOUDFLARE_ACCOUNT_ID (or put it in .r2.env)." >&2
  exit 1
fi

export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION=auto
ENDPOINT="https://${CLOUDFLARE_ACCOUNT_ID}.r2.cloudflarestorage.com"

echo "=== Upload to s3://siroc-cp ($VER) ==="
aws s3 cp "$PKG" "s3://siroc-cp/siroc-linux-amd64.tar.gz" \
  --endpoint-url "$ENDPOINT" \
  --content-type application/octet-stream \
  --cache-control "public, max-age=60"
aws s3 cp "$PKG" "s3://siroc-cp/${VER}/siroc-linux-amd64.tar.gz" \
  --endpoint-url "$ENDPOINT" \
  --content-type application/octet-stream \
  --cache-control "public, max-age=31536000, immutable"
aws s3 cp "$META" "s3://siroc-cp/latest.json" \
  --endpoint-url "$ENDPOINT" \
  --content-type application/json \
  --cache-control "no-cache"

if [ "$SKIP_PRUNE" -eq 1 ]; then
  echo "Done. https://get.siroc.dev/latest.json"
  exit 0
fi

echo "Keeping the 3 newest version prefixes"
aws s3api list-objects-v2 \
  --bucket siroc-cp \
  --delimiter / \
  --endpoint-url "$ENDPOINT" \
  --query 'CommonPrefixes[].Prefix' \
  --output text > "$ROOT/dist/r2-prefixes.txt" || true
python3 "$ROOT/scripts/prune-r2-versions.py" 3 < "$ROOT/dist/r2-prefixes.txt" > "$ROOT/dist/r2-drop.txt"
if [ ! -s "$ROOT/dist/r2-drop.txt" ]; then
  echo "Nothing to delete"
  echo "Done. https://get.siroc.dev/latest.json"
  exit 0
fi
while IFS= read -r prefix; do
  [ -n "$prefix" ] || continue
  echo "Deleting s3://siroc-cp/${prefix}"
  aws s3 rm "s3://siroc-cp/${prefix}" --recursive --endpoint-url "$ENDPOINT"
done < "$ROOT/dist/r2-drop.txt"
echo "Done. https://get.siroc.dev/latest.json"
