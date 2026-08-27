#!/usr/bin/env bash
set -Eeuo pipefail

DATA_DIR=/var/lib/wg-route-panel
install -d -m 0750 -o wgpanel -g wgpanel \
  "$DATA_DIR" "$DATA_DIR/wireguard" "$DATA_DIR/sing-box" "$DATA_DIR/xray" "$DATA_DIR/geosite"

if [[ ! -s "$DATA_DIR/sing-box/config.json" ]]; then
  printf '%s\n' '{"log":{"level":"info","timestamp":true}}' >"$DATA_DIR/sing-box/config.json"
fi
if [[ ! -s "$DATA_DIR/xray/config.json" ]]; then
  cp /usr/share/wg-route-panel/xray.json "$DATA_DIR/xray/config.json"
fi
chown -R wgpanel:wgpanel "$DATA_DIR"
chmod 0600 "$DATA_DIR/sing-box/config.json"
chmod 0640 "$DATA_DIR/xray/config.json"

/usr/local/bin/sing-box check -c "$DATA_DIR/sing-box/config.json"
/usr/local/bin/xray run -test -config "$DATA_DIR/xray/config.json"

if [[ $# -gt 0 ]]; then
  exec /usr/sbin/runuser -u wgpanel -- /usr/local/bin/wg-route-panel "$@"
fi
exec /usr/bin/supervisord -c /etc/supervisor/conf.d/wg-route-panel.conf
