#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
docker compose down --remove-orphans 2>/dev/null || true
if [[ $EUID -eq 0 ]]; then
  nft delete table inet wg_route_panel 2>/dev/null || true
else
  sudo nft delete table inet wg_route_panel 2>/dev/null || true
fi
if [[ "${1:-}" == "--purge" ]]; then
  rm -rf data .env
  echo "Containers, local image data, configuration, and secrets removed."
else
  echo "Containers removed; ./data and .env retained. Use --purge to delete them."
fi
