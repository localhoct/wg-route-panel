#!/usr/bin/env bash
set -u
fail=0
check() { if "$@" >/dev/null 2>&1; then printf '[OK] %s\n' "$*"; else printf '[WARN] %s\n' "$*"; fail=1; fi; }
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
[[ -r /etc/os-release ]] && . /etc/os-release && echo "OS: ${PRETTY_NAME:-unknown}"
check command -v docker
check docker compose version
check docker info
check test -c /dev/net/tun
check command -v nft
check test -f configs/panel.docker.yaml
check test -f docker-compose.yml
# 53 (DNS) is expected to be in use once the stack is running, since sing-box
# listens on it intentionally on the public interface; 3389 (panel) and 1080
# (SOCKS, when enabled) are the other host-network ports this stack binds.
for port in 53 1080 3389; do
  if ss -lntup 2>/dev/null | grep -qE ":${port}\\b"; then
    echo "[INFO] port $port is in use"
  else
    echo "[OK] port $port available"
  fi
done
if docker compose ps --status running 2>/dev/null | grep -q wg-route-panel; then
  check curl -fsS http://127.0.0.1:3389/healthz
  docker compose exec -T panel /usr/local/bin/sing-box version || fail=1
  docker compose exec -T panel /usr/bin/supervisorctl status || fail=1
else
  echo "[INFO] stack is not running"
fi
exit "$fail"
