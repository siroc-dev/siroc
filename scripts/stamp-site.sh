#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SITE="$ROOT/site"
VER="$(tr -d '[:space:]' <"$ROOT/VERSION" 2>/dev/null || echo 0.0.0)"
RELEASE_BASE="${SIROC_RELEASE_PUBLIC_BASE:-https://releases.siroc.dev}"
SITE_BASE="${SIROC_SITE_PUBLIC_BASE:-https://siroc.pages.dev}"
RELEASE_BASE="${RELEASE_BASE%/}"
SITE_BASE="${SITE_BASE%/}"

replace() {
  local file="$1"
  local tmp
  tmp="$(mktemp)"
  sed -e "s|__SIROC_RELEASE_BASE__|${RELEASE_BASE}|g" \
      -e "s|__SIROC_SITE_BASE__|${SITE_BASE}|g" \
      -e "s|id=\"version\">[^<]*<|id=\"version\">${VER}<|g" \
      "$file" >"$tmp"
  mv "$tmp" "$file"
}

replace "$SITE/install.sh"
replace "$SITE/app.js"
replace "$SITE/index.html"

# Keep a committed-looking install command if the placeholder is still there.
if grep -q "__SIROC_SITE_BASE__" "$SITE/index.html"; then
  sed -i "s|__SIROC_SITE_BASE__|${SITE_BASE}|g" "$SITE/index.html"
fi

echo "Stamped site version=$VER release=$RELEASE_BASE site=$SITE_BASE"
