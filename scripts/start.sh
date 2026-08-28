#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

build=true
configure_firewall=true

usage() {
  cat <<'USAGE'
Usage: scripts/start.sh [options]

Build and start WG Route Panel with Docker Compose.

Options:
  --no-build        Start the existing image without rebuilding it
  --skip-firewall   Do not validate or initialize the nftables policy
  -h, --help        Show this help

There is no administrator-creation step here anymore: the first time you
open the panel in a browser with no administrator yet configured, it shows
a Setup Wizard (at /setup) to create one. Nothing to do over SSH.
USAGE
}

die() {
  echo "Error: $*" >&2
  exit 1
}

run_root() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "root privileges are required for nftables; rerun with sudo or use --skip-firewall"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build) build=false ;;
    --skip-firewall) configure_firewall=false ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown option: $1" ;;
  esac
  shift
done

command -v docker >/dev/null 2>&1 || die "Docker is not installed; run sudo ./scripts/install.sh first"
command -v curl >/dev/null 2>&1 || die "curl is required for the health check"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"
docker info >/dev/null 2>&1 || die "cannot access the Docker daemon; start Docker or rerun this script with sudo"
[[ -c /dev/net/tun ]] || die "/dev/net/tun is unavailable"
[[ -f docker-compose.yml ]] || die "docker-compose.yml is missing"
[[ -f configs/panel.docker.yaml ]] || die "configs/panel.docker.yaml is missing"

install -d -m 0750 data
if [[ ! -f .env ]]; then
  command -v openssl >/dev/null 2>&1 || die "openssl is required to create .env"
  umask 077
  printf 'PANEL_SESSION_SECRET=%s\n' "$(openssl rand -base64 48)" > .env
  echo "Created .env with a random session secret."
fi
chmod 0600 .env
if ! grep -Eq '^PANEL_SESSION_SECRET=.{32,}$' .env; then
  die ".env must contain PANEL_SESSION_SECRET with at least 32 characters"
fi

if $configure_firewall; then
  command -v nft >/dev/null 2>&1 || die "nftables is required; install it or use --skip-firewall"
  run_root nft -c -f deploy/nftables/wg-route-panel.nft
  if ! run_root nft list table inet wg_route_panel >/dev/null 2>&1; then
    run_root nft -f deploy/nftables/wg-route-panel.nft
    echo "Initialized nftables table inet wg_route_panel."
  fi
fi

if $build; then
  docker compose build --pull
fi

docker compose up -d

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:3389/healthz >/dev/null 2>&1; then
    docker compose ps
    cat <<'READY'
WG Route Panel is ready.

Open http://127.0.0.1:3389/setup in a browser to create the first
administrator account (only shown until one exists) - no SSH needed.
For remote/LAN access to the panel itself, allow the client CIDR in both
panel_allow and api_allow (see the panel's Access Control page, or
./scripts/nft-element.sh). DNS on port 53 is public by design and needs no
ACL entry. Do not expose the plain-HTTP panel port to the public internet;
put HTTPS (Caddy/Nginx) in front of it first.
READY
    exit 0
  fi
  sleep 2
done

docker compose ps
docker compose logs --tail=100 panel >&2
die "the panel did not become healthy on port 3389"
