# WG Route Panel

WG Route Panel is a secure, self-hosted administration panel for a **sing-box userspace WireGuard endpoint**, Xray-core SOCKS5/DNS routing, geosite categories, and nftables source allow-lists. The primary deployment is one Docker Compose service running the Go panel, sing-box, and Xray under Supervisor.

> Use this software only for lawful, authorized networking. Operators are responsible for applicable laws, provider policies, content restrictions, and client consent.

## Features

- Accepts familiar wg-quick-style WireGuard configuration, performs semantic validation, converts it to sing-box JSON, and runs `sing-box check` before activation.
- sing-box owns the WireGuard protocol and creates the `wg0` system interface; `wg-quick` and kernel WireGuard lifecycle services are not used.
- Supervisor-backed start, stop, restart, and process health for sing-box and Xray.
- Xray SOCKS5 on `127.0.0.1:1080` and DNS adapter on `127.0.0.1:5353` by default.
- Xray direct, block, static-domain, geosite, and WireGuard-pinned routes through `sockopt.interface=wg0`.
- nftables IPv4/IPv6 source allow-lists for panel/API, DNS, and SOCKS.
- bcrypt cost-12 passwords, optional TOTP, hashed opaque sessions, strict cookies, CSRF, login throttling, security headers, and audit logs.
- SQLite, server-rendered Go templates, HTMX-compatible endpoints, and a responsive Bootstrap UI.
- Pinned, SHA-256-verified sing-box and Xray artifacts in a multi-stage Docker image.

## Architecture

```text
Browser/API --HTTP:9090--> host-network Docker container
                                         |-- Go panel (user wgpanel) --> SQLite
                                         |-- Supervisor
                                         |    |-- sing-box --> userspace WireGuard --> wg0 system interface
                                         |    `-- Xray SOCKS/DNS --> direct | block | bind interface wg0
                                         `-- nft command allow-list --> host network namespace
Clients -------- host nftables ACL -------> Xray SOCKS/DNS
```

The container deliberately uses `network_mode: host`. That lets the sing-box-created `wg0` interface and nftables rules operate in the host network namespace, and lets Xray bind selected traffic to `wg0`. It also means Compose port publishing is not used: the panel listens directly on **TCP port 9090**. The container receives only `NET_ADMIN` plus `/dev/net/tun`; it is not privileged.

sing-box 1.13.19 and Xray 26.3.27 are pinned in `Dockerfile`. Xray remains the DNS/SOCKS/geosite routing layer because modern sing-box releases removed the legacy geosite workflow.

## Requirements

- Ubuntu Server 22.04 or 24.04.
- Docker Engine with Docker Compose v2.
- `/dev/net/tun` and permission to add `NET_ADMIN` to the container.
- nftables on the host when panel-managed ACLs are enabled.
- Free host ports: panel `9090/tcp`, SOCKS `1080/tcp`, and DNS `5353/tcp+udp` by default.

No host Go toolchain, `wireguard-tools`, `wg-quick`, sing-box, or Xray installation is required.

## Quick start

```bash
git clone https://github.com/localhoct/wg-route-panel.git
cd wg-route-panel
sudo ./scripts/install.sh

# Create the only administrator without committing or storing its password.
sudo env PANEL_ADMIN_PASSWORD_INIT='replace-with-a-long-random-password' \
  docker compose run --rm panel create-admin

sudo docker compose up -d
sudo docker compose ps
curl http://127.0.0.1:9090/healthz
```

`install.sh` installs Docker/nftables on supported Ubuntu releases, creates a mode-`0600` `.env` containing a random session secret, validates the nftables policy, and builds the image. Review `.env` and `configs/panel.docker.yaml` before starting production.

Do not leave the initial password in shell history. Without `PANEL_ADMIN_PASSWORD_INIT`, `create-admin` prompts on standard input.

## HTTP on port 9090

The included Docker configuration listens on `0.0.0.0:9090` over plain HTTP with `allow_insecure_http: true` and `secure_cookies: false`. Open `http://127.0.0.1:9090` on the server after starting Compose. For another LAN device, first add that device or subnet to both the `panel_allow` and `api_allow` nftables sets. This explicit opt-in is suitable only for a trusted LAN or temporary setup.

When you are ready for HTTPS, place Caddy or Nginx in front of it. Example Caddy configuration:

```caddy
panel.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:9090
}
```

For HTTPS, set `base_url: "https://panel.example.com"`, change `secure_cookies` to `true`, and remove or disable `allow_insecure_http`. The health endpoint is intentionally unauthenticated and returns only `{ "status": "ok" }`.

Never expose plain HTTP across the public internet because credentials and session cookies can be intercepted.

## WireGuard through sing-box

Open **WireGuard via sing-box** and upload or paste an INI file containing:

- `[Interface]`: `Address`, `PrivateKey`, and optional `ListenPort`/`MTU`.
- One or more `[Peer]`: `PublicKey`, `AllowedIPs`, and `Endpoint`; optional `PresharedKey` and `PersistentKeepalive`.

Example shape (replace every placeholder):

```ini
[Interface]
Address = 10.0.0.2/32
PrivateKey = <private-key>
MTU = 1408

[Peer]
PublicKey = <peer-public-key>
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:51820
PersistentKeepalive = 25
```

The panel stores the source at `data/wireguard/wg0.conf`, generates `data/sing-box/config.json`, and invokes `sing-box check` against a temporary candidate before replacing the active JSON. Both files use mode `0600`. Hostname endpoints receive a local DNS resolver in the generated sing-box configuration. Start or restart sing-box from the UI after saving.

Health combines Supervisor state, existence of `wg0`, RX/TX counters from sysfs, and an optional `wireguard.probe_address` ping through `wg0`. sing-box does not expose `wg show latest-handshakes`; the panel reports its runtime ownership instead.

## SOCKS5, DNS, and geosite

- SOCKS test: `curl --socks5-hostname 127.0.0.1:1080 https://ifconfig.me`
- DNS test: `dig @127.0.0.1 -p 5353 example.com`
- `proxy-route` rules use the Xray freedom outbound bound to the sing-box-created `wg0` interface.
- DNS rules affect resolution/routing decisions; they do not transparently proxy arbitrary client traffic.
- For any non-loopback SOCKS bind, both `PANEL_SOCKS_USERNAME` and `PANEL_SOCKS_PASSWORD` are mandatory. Add them to the protected `.env` and apply a source ACL.
- Update `geosite.dat` through the panel or `./scripts/geosite-update.sh`; configure `geosite.sha256` when an expected digest is available.

## ACL and nftables

`deploy/nftables/wg-route-panel.nft` creates `table inet wg_route_panel`. It defaults to loopback-only access for 9090, 1080, and 5353 and does not change SSH policy. Review it against the host's existing firewall before applying it.

```bash
sudo nft -c -f deploy/nftables/wg-route-panel.nft
sudo nft -f deploy/nftables/wg-route-panel.nft

# Example: allow one LAN administrator to reach the HTTP panel and API.
sudo nft add element inet wg_route_panel panel_allow '{ 192.168.1.50/32 }'
sudo nft add element inet wg_route_panel api_allow '{ 192.168.1.50/32 }'
sudo nft list table inet wg_route_panel
```

When `firewall.enabled` is true, the panel incrementally updates only predefined sets through a tightly constrained sudo rule inside the container. Because host networking is used, these rules affect host traffic.

## Operations

```bash
make docker-build
make docker-up
make docker-logs
make doctor
make docker-down

# Direct equivalents
docker compose build
docker compose up -d
docker compose logs -f panel
docker compose exec panel supervisorctl status
docker compose exec panel sing-box check -c /var/lib/wg-route-panel/sing-box/config.json
docker compose exec panel xray run -test -config /var/lib/wg-route-panel/xray/config.json
```

Supervisor starts the panel as unprivileged `wgpanel`, sing-box as root with the container's `NET_ADMIN`, and Xray as `wgpanel`. The panel communicates with Supervisor through a group-restricted Unix socket.

## Configuration

The production Compose configuration is `configs/panel.docker.yaml`; `docker-compose.yml` bind-mounts it read-only. Persistent runtime files are under `./data`.

Important environment values in `.env`:

```dotenv
PANEL_SESSION_SECRET=<at-least-32-random-characters>
PANEL_SOCKS_USERNAME=
PANEL_SOCKS_PASSWORD=
```

Never commit `.env`, WireGuard keys, database files, TOTP seeds, backups, or production configuration containing private data.

## Backup, restore, and uninstall

```bash
./scripts/backup.sh /secure/wg-route-panel-$(date +%F).tar.gz
./scripts/restore.sh /secure/wg-route-panel-2026-08-27.tar.gz
./scripts/uninstall.sh          # retain ./data and .env
./scripts/uninstall.sh --purge  # permanently delete local state and secrets
```

Backups include private keys, password hashes, session secrets, and configuration. They are created mode `0600`; encrypt them with age/GPG before off-host storage and test restoration.

## Development and verification

```bash
export PATH="$PWD/.tooling/go/bin:$PATH" # only if using the optional local toolchain
make lint
make test
make build

docker compose config
PANEL_SESSION_SECRET="$(openssl rand -base64 48)" docker compose build
```

The repository CI workflow runs formatting, vet, race tests, and a Go build. Validate Compose locally with the commands above; Docker workflow changes are intentionally not part of this pull request because the connected GitHub App cannot update workflow files.

## Security notes

- Plain HTTP on 9090 is enabled only as an explicit temporary opt-in; limit it to localhost or trusted source ACLs and enable HTTPS before internet exposure.
- Host networking and `NET_ADMIN` are security-sensitive. Keep the image pinned, inspect updates, and never add `privileged: true`.
- Restrict Docker daemon access; membership in the Docker group is effectively root access.
- Use MFA, long unique credentials, SSH keys, a management network, automatic security updates, and a recovery console.
- Audit container capabilities, listening sockets, nftables, `.env` permissions, and persistent file ownership after upgrades.
- The application never interpolates uploaded data into a shell command.

## License

MIT. No warranty. Operators retain responsibility for secure and lawful deployment.
