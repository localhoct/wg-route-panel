# WG Route Panel

WG Route Panel is a secure, self-hosted administration panel for a **sing-box-only** data plane: sing-box alone runs the userspace WireGuard tunnel, the SOCKS5/HTTP proxy, DNS interception/resolution, and geosite-based routing. There is no Xray-core anywhere in the stack. The primary deployment is one Docker Compose service running the Go panel and sing-box under Supervisor.

> Use this software only for lawful, authorized networking. Operators are responsible for applicable laws, provider policies, content restrictions, and client consent.

## Design principles

- **Everything from the browser.** Creating the first administrator, uploading a WireGuard config, changing DNS/SOCKS listener settings and credentials, and selecting geosite categories are all done from the web panel. You never need to SSH into the server for routine administration.
- **DNS is public by default.** The whole point of running this panel is that clients out on the internet can point their DNS at this server's public IP and have it resolve/route/block through the sing-box tunnel. Port 53 has no allow-list and is not gated by nftables — that's intentional, not an oversight.
- **Everything else is default-deny remotely.** The panel/API (port 9090) and the SOCKS5 proxy (port 1080, off by default) are loopback-only until an operator explicitly adds a source CIDR from the panel's Access Control page.
- **sing-box does it all.** A single sing-box process owns the WireGuard endpoint, the `mixed` SOCKS4/5+HTTP inbound, the DNS-intercepting inbounds, and geosite-based routing via SagerNet's remote `.srs` rule-sets — no separate DNS/proxy process, no local geo-data downloads.

## Features

- Accepts a familiar wg-quick-style WireGuard `.conf`, validates it, converts it into a sing-box `wireguard` **endpoint** (not the older `outbound` style — the endpoint's tag can be referenced directly as a `route.rules[].outbound` target), and runs `sing-box check` on a candidate file before activating it.
- sing-box owns the WireGuard protocol and creates the system `wg0` interface itself; `wg-quick`/kernel WireGuard services are not used.
- A **Setup Wizard** at `/setup` creates the first administrator account from the browser; it is only reachable until an administrator exists.
- DNS and SOCKS5 listener addresses/ports/upstreams/credentials are stored in SQLite and edited from the panel's DNS and SOCKS5 pages — no YAML editing, no container exec.
- Per-domain DNS rules (`direct`, `proxy-route` through the tunnel, `block`, `static` IP override) and geosite category selection (SagerNet `sing-geosite` tags, validated by an HTTP `HEAD` against the real rule-set repository — no local downloading or parsing by the panel itself).
- Supervisor-backed start/stop/restart and health reporting for the sing-box process.
- nftables IPv4/IPv6 source allow-lists for the panel/API and SOCKS5 — but **not** for DNS, which is intentionally public.
- bcrypt cost-12 passwords (change requires the current password), optional TOTP, hashed opaque sessions, strict cookies, CSRF protection, login throttling, security headers, and an audit log.
- SQLite storage, server-rendered Go templates, HTMX-compatible endpoints, and a responsive Bootstrap UI.
- A pinned, SHA-256-verified sing-box binary in a multi-stage Docker image.

## Architecture

```text
Public internet ---- UDP/TCP :53 (no ACL, by design) ----> sing-box DNS inbounds --sniff+hijack-dns--> dns rules / geosite / route
Browser/API --HTTP:9090--> host-network Docker container
                                         |-- Go panel (user wgpanel) --> SQLite (rules, settings, users, audit log)
                                         `-- Supervisor
                                              `-- sing-box (root, NET_ADMIN)
                                                   |-- wireguard endpoint --> wg0 system interface
                                                   |-- mixed inbound (SOCKS5/SOCKS4/HTTP) --> route rules
                                                   `-- direct inbounds :53 --> DNS sniff/hijack --> dns rules / geosite rule-sets
Clients -------- host nftables ACL (panel/api/socks only) -------> panel:9090 / socks:1080
```

The container uses `network_mode: host` so the sing-box-created `wg0` interface and nftables rules operate in the host network namespace. Compose port publishing is not used: the panel listens directly on **TCP port 9090** and sing-box listens directly on the ports configured from the DNS/SOCKS5 pages (**UDP/TCP 53** by default for DNS, **TCP 1080** for SOCKS5 when enabled). The container receives only `NET_ADMIN` plus `/dev/net/tun`; it is not privileged.

sing-box 1.13.19 is pinned and SHA-256-verified in `Dockerfile`.

## Requirements

- Ubuntu Server 22.04 or 24.04.
- Docker Engine with Docker Compose v2.
- `/dev/net/tun` and permission to add `NET_ADMIN` to the container.
- nftables on the host when panel-managed ACLs are enabled.
- Free host ports: panel `9090/tcp`, DNS `53/tcp+udp` (public by design), and SOCKS `1080/tcp` if you enable it.

No host Go toolchain, `wireguard-tools`, `wg-quick`, or sing-box installation is required — everything ships inside the Docker image.

## Quick start

```bash
git clone https://github.com/localhoct/wg-route-panel.git
cd wg-route-panel
sudo ./scripts/install.sh

# Build the image, apply nftables, and start Compose.
sudo ./scripts/start.sh
```

`install.sh` installs Docker/nftables on supported Ubuntu releases, creates a mode-`0600` `.env` containing a random session secret, validates the nftables policy, and builds the image. `start.sh` validates Docker, Compose, `/dev/net/tun`, and nftables; builds the image; starts the stack; and waits for the health endpoint. There is **no admin-creation step in this script anymore** — that happens once, in the browser.

Once the script reports the panel is healthy:

1. Open `http://<server-ip-or-127.0.0.1>:9090/setup` in a browser.
2. Create the first administrator (username + password, minimum 12 characters). This page disappears once an administrator exists.
3. Sign in at `/login` and continue setup from the dashboard (see "How it works" below).

Run `./scripts/start.sh --help` for `--no-build` and `--skip-firewall` options. A user with Docker daemon access may run the script without `sudo`; nftables setup will use `sudo` when necessary.

### Disaster-recovery admin creation (CLI, optional)

If every administrator account somehow becomes unusable (locked out, forgotten password with no recovery, or a database restored without any user), you can create an additional administrator from the container without a browser:

```bash
docker compose run --rm panel create-admin myrecoveryadmin
```

This is a fallback only — the supported day-one flow is the `/setup` wizard in the browser.

## HTTP on port 9090

The included Docker configuration listens on `0.0.0.0:9090` over plain HTTP with `allow_insecure_http: true` and `secure_cookies: false`. Open `http://127.0.0.1:9090` on the server after starting Compose. For another device, first add its address/subnet to both `panel_allow` and `api_allow` from the panel's **Access Control** page (or with `nft`/`scripts/nft-element.sh` directly). This explicit opt-in is suitable only for a trusted LAN, an SSH tunnel, or a temporary setup.

When you are ready for HTTPS, place Caddy or Nginx in front of it. Example Caddy configuration:

```caddy
panel.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:9090
}
```

For HTTPS, set `base_url: "https://panel.example.com"` in `configs/panel.docker.yaml`, change `secure_cookies` to `true`, and remove or disable `allow_insecure_http`. The health endpoint (`/healthz`) is intentionally unauthenticated and returns only `{ "status": "ok" }`.

Never expose the plain-HTTP panel across the public internet because credentials and session cookies can be intercepted. (This restriction is specific to the *panel* on port 9090 — the *DNS* service on port 53 is meant to be public, see below.)

## How it works, end to end

Everything below is done from the web panel after signing in — no SSH, no editing files inside the container.

### 1. Connect WireGuard through sing-box

Open **WireGuard** in the left nav and upload or paste an INI file containing:

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

Saving this:

1. Validates the file, then stores the source at `data/wireguard/wg0.conf` (mode `0600`).
2. Regenerates the single unified sing-box configuration (`data/sing-box/config.json`), which now includes a `wireguard` **endpoint** built from this config alongside the SOCKS/DNS inbounds and every DNS rule/geosite selection already configured.
3. Runs `sing-box check` against a temporary candidate file; only if that succeeds does it atomically replace the active configuration.

Start or restart sing-box from the WireGuard page after saving. The dashboard's connection status combines Supervisor's process state, the existence of the `wg0` interface, RX/TX counters read from sysfs, and (if you set a `wireguard.probe_address` in `configs/panel.docker.yaml`) a ping through `wg0`. sing-box has no `wg show latest-handshakes` equivalent, so the panel reports its own runtime ownership of the interface instead of a raw handshake timestamp.

### 2. Configure DNS (public by default)

Open the **DNS** page. The top form controls the actual sing-box listener:

- **Listen address** — defaults to `0.0.0.0`, meaning DNS answers on **every interface, including the server's public IP**, with no extra firewall setup. This is what makes the DNS service usable by any client on the internet, which was the whole point of this feature.
- **Port** — defaults to `53`.
- **Direct upstream** / **Proxy-route upstream** — the resolvers sing-box itself queries when a domain isn't matched by a `block`/`static` rule; the proxy-route upstream is queried *through* the WireGuard tunnel (`detour` to the `wireguard` endpoint) whenever WireGuard peers are configured.

Below that, add per-domain rules:

| Action | Effect |
|---|---|
| `direct` | Resolve via the direct upstream; route the connection outside the tunnel. |
| `proxy-route` | Resolve via the proxy upstream *through* WireGuard; route the connection through the tunnel. |
| `block` | DNS query is rejected (`action: reject`) — the domain will not resolve at all. |
| `static` | Answer with a fixed IP you provide, bypassing any upstream. |

Saving any of this — the listener settings or a rule — regenerates and re-validates the sing-box configuration automatically; there is nothing further to apply.

Test resolution directly against the listener:

```bash
dig @<server-ip> -p 53 example.com
```

### 3. Configure SOCKS5 (off by default, loopback by default)

Open the **SOCKS5** page. sing-box's built-in `mixed` inbound handles SOCKS4, SOCKS5, and HTTP proxying on one listener — no separate process. Turn it on with the checkbox, set the listen address/port, and:

- If you bind it to anything other than `127.0.0.1`/`::1`, a username and password become **mandatory** (enforced by the panel) so the proxy can't be used by an arbitrary host on the network.
- Unlike DNS, SOCKS5 is **not** public by default: add the client's CIDR to the `socks` scope on the **Access Control** page before it will be reachable from outside loopback, even with credentials set.

Test it:

```bash
curl --socks5-hostname <username>:<password>@<server-ip>:1080 https://ifconfig.me
```

### 4. Route by geosite category

Open the **Geosite** page. Enter any tag from [SagerNet/sing-geosite](https://github.com/SagerNet/sing-geosite) (e.g. `category-ads-all`, `geolocation-cn`) and choose `direct`, `proxy-route`, or `block`. The panel validates the tag with an HTTP `HEAD` request against SagerNet's rule-set repository (200 = known tag, 404 = unknown) — it never downloads or parses the geo data itself; sing-box fetches and caches the compiled `.srs` rule-set directly at runtime. Saving a selection regenerates the sing-box configuration the same way DNS rules do.

### 5. Lock down remote access (Access Control)

Open **Access Control**. This page manages the nftables allow-lists for the **panel/API** and **SOCKS5** scopes only — DNS has no entry here because it's public by design (this is explained on the page itself). Add a source CIDR (IPv4 or IPv6) per scope; the panel applies it to the corresponding `*_allow`/`*_allow6` nftables set through a tightly constrained `sudo` rule inside the container. These rules never touch SSH.

### 6. Everything else

- **Settings**: change your password (the current password is now required — this closes a bug where a hijacked session could silently take over the account).
- **Logs**: the audit trail of every action above, including the acting user's IP.

## ACL and nftables

`deploy/nftables/wg-route-panel.nft` creates `table inet wg_route_panel`. It defaults to loopback-only access for the panel (9090) and SOCKS5 (1080), and does **not** touch DNS (53) or SSH policy at all. Review it against the host's existing firewall before applying it.

```bash
sudo nft -c -f deploy/nftables/wg-route-panel.nft
sudo nft -f deploy/nftables/wg-route-panel.nft

# Example: allow one LAN administrator to reach the HTTP panel and API.
sudo nft add element inet wg_route_panel panel_allow '{ 192.168.1.50/32 }'
sudo nft add element inet wg_route_panel api_allow '{ 192.168.1.50/32 }'
sudo nft list table inet wg_route_panel
```

Prefer doing this from the panel's Access Control page — it uses the same restricted `sudo` path and gets logged to the audit trail. The commands above are for the initial host-level bootstrap or manual recovery.

## Operations

```bash
make start
make docker-build
make docker-up
make docker-logs
make doctor
make docker-down

# Direct equivalents
./scripts/start.sh
docker compose build
docker compose up -d
docker compose logs -f panel
docker compose exec panel supervisorctl status
docker compose exec panel sing-box check -c /var/lib/wg-route-panel/sing-box/config.json
```

Supervisor starts the panel as unprivileged `wgpanel` and sing-box as root with the container's `NET_ADMIN`. The panel communicates with Supervisor through a group-restricted Unix socket.

## Configuration

The production Compose configuration is `configs/panel.docker.yaml`; `docker-compose.yml` bind-mounts it read-only. Persistent runtime files are under `./data`. This YAML file only holds bootstrap/infrastructure settings that must exist before the database is available — the listen address, session secret, binary paths, and logging. **Everything an operator tunes routinely (DNS/SOCKS listener settings and credentials, geosite selections, per-domain DNS rules, ACLs, the administrator account) lives in SQLite and is managed entirely from the web panel.**

The only environment value in `.env`:

```dotenv
PANEL_SESSION_SECRET=<at-least-32-random-characters>
```

Never commit `.env`, WireGuard keys, database files, TOTP seeds, backups, or production configuration containing private data.

## Backup, restore, and uninstall

```bash
./scripts/backup.sh /secure/wg-route-panel-$(date +%F).tar.gz
./scripts/restore.sh /secure/wg-route-panel-2026-08-27.tar.gz
./scripts/uninstall.sh          # retain ./data and .env
./scripts/uninstall.sh --purge  # permanently delete local state and secrets
```

Backups include private keys, password hashes, session secrets, and configuration (the SQLite database now holds DNS/SOCKS settings and geosite selections too, so a single backup captures the entire operator-tunable state). They are created mode `0600`; encrypt them with age/GPG before off-host storage and test restoration.

## Development and verification

```bash
export PATH="$PWD/.tooling/go/bin:$PATH" # only if using the optional local toolchain
make lint
make test
make build

docker compose config
PANEL_SESSION_SECRET="$(openssl rand -base64 48)" docker compose build
```

The repository CI workflow runs formatting, vet, race tests, and a Go build.

## Security notes

- Plain HTTP on 9090 is enabled only as an explicit temporary opt-in for the *panel*; limit it to localhost or trusted source ACLs and enable HTTPS before internet exposure. This does **not** apply to DNS on port 53, which is meant to be public.
- Host networking and `NET_ADMIN` are security-sensitive. Keep the image pinned, inspect updates, and never add `privileged: true`.
- Restrict Docker daemon access; membership in the Docker group is effectively root access.
- Use MFA (TOTP), long unique credentials, SSH keys, a management network, automatic security updates, and a recovery console.
- Audit container capabilities, listening sockets, nftables, `.env` permissions, and persistent file ownership after upgrades.
- The application never interpolates uploaded data into a shell command.

## License

MIT. No warranty. Operators retain responsibility for secure and lawful deployment.
