#!/usr/bin/env bash
set -Eeuo pipefail
[[ $# -eq 3 ]] || { echo "usage: $0 add|delete set cidr" >&2; exit 2; }
op=$1
set_name=$2
cidr=$3
[[ "$op" == add || "$op" == delete ]] || { echo "invalid operation" >&2; exit 2; }
case "$set_name" in
  dns_allow|dns_allow6|panel_allow|panel_allow6|socks_allow|socks_allow6|api_allow|api_allow6) ;;
  *) echo "invalid set" >&2; exit 2 ;;
esac
[[ "$cidr" =~ ^[0-9A-Fa-f:.]+/[0-9]{1,3}$ ]] || { echo "invalid CIDR" >&2; exit 2; }
exec /usr/sbin/nft "$op" element inet wg_route_panel "$set_name" "{" "$cidr" "}"
