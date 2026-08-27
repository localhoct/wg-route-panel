#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
out="${1:-$ROOT/wg-route-panel-$(date +%F-%H%M%S).tar.gz}"
install -d -m 0700 "$(dirname "$out")"
items=(data configs/panel.docker.yaml docker-compose.yml)
[[ -f .env ]] && items+=(.env)
tar -czf "$out" "${items[@]}"
chmod 0600 "$out"
echo "$out (sensitive: contains keys, hashes, and possibly credentials; encrypt before off-host storage)"
