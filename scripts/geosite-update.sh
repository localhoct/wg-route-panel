#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
url="${1:-https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat}"
out="${2:-$ROOT/data/geosite/geosite.dat}"
[[ "$url" == https://* ]] || { echo "HTTPS required" >&2; exit 1; }
mkdir -p "$(dirname "$out")"
tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT
curl --proto '=https' --tlsv1.2 -fL "$url" -o "$tmp"
[[ -z "${GEOSITE_SHA256:-}" ]] || echo "$GEOSITE_SHA256  $tmp" | sha256sum -c -
install -m 0640 "$tmp" "$out"
echo "Updated $out; restart Xray after selecting routes in the panel."
