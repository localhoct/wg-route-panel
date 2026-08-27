#!/usr/bin/env bash
set -Eeuo pipefail
[[ -f "${1:-}" ]] || { echo "Usage: $0 backup.tar.gz" >&2; exit 1; }
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
archive="$(realpath "$1")"
cd "$ROOT"
if tar -tzf "$archive" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
  echo "Unsafe backup paths" >&2
  exit 1
fi
tar -tzf "$archive" | grep -Eq '^data/|^configs/panel\.docker\.yaml$' || { echo "Invalid backup" >&2; exit 1; }
docker compose down 2>/dev/null || true
tar -xzf "$archive" -C "$ROOT"
chmod 0600 .env 2>/dev/null || true
docker compose up -d
