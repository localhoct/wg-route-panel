#!/usr/bin/env bash
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
. /etc/os-release
[[ "$ID" == ubuntu && ("$VERSION_ID" == "22.04" || "$VERSION_ID" == "24.04") ]] || { echo "Ubuntu 22.04/24.04 required" >&2; exit 1; }
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io nftables curl ca-certificates openssl
if ! docker compose version >/dev/null 2>&1; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y docker-compose-v2 \
    || DEBIAN_FRONTEND=noninteractive apt-get install -y docker-compose-plugin \
    || { echo "Docker Compose v2 is required" >&2; exit 1; }
fi
systemctl enable --now docker nftables
install -d -m 0750 data
if [[ ! -f .env ]]; then
  umask 077
  printf 'PANEL_SESSION_SECRET=%s\n' "$(openssl rand -base64 48)" >.env
  chmod 0600 .env
fi

nft -c -f deploy/nftables/wg-route-panel.nft
if ! nft list table inet wg_route_panel >/dev/null 2>&1; then
  nft -f deploy/nftables/wg-route-panel.nft
fi

docker compose build --pull
cat <<NEXT
Docker image built. Next:
1. Review configs/panel.docker.yaml and .env (mode 0600).
2. Create the administrator:
   PANEL_ADMIN_PASSWORD_INIT='a-long-random-password' docker compose run --rm panel create-admin
3. Start the stack: docker compose up -d
4. Open http://127.0.0.1:9090 locally. For remote access, add your source CIDR to panel_allow and api_allow or use an SSH tunnel.
5. Check health: docker compose ps && curl http://127.0.0.1:9090/healthz
6. Add HTTPS later before exposing port 9090 to the public internet.
NEXT
