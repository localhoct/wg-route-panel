# WG Route Panel

A secure, self-hosted Ubuntu control panel for managing WireGuard, Xray-core (SOCKS5/DNS), and nftables ACLs.

## ⚠️ Legal & Safety Warning
Users must comply with all applicable local laws, service terms, and provider policies. This tool is designed for secure, lawful routing and privacy management. Do not use for illegal activities.

## Features
- Linux Kernel WireGuard management (wg-quick)
- Xray-core SOCKS5 proxy & DNS routing with geosite support
- nftables-based strict IP allow-listing (ACL)
- Responsive HTMX + Tailwind CSS Admin UI
- SQLite database with Argon2id password hashing
- Systemd integration & least-privilege execution

## Quick Start
1. Clone the repository: `git clone https://github.com/yourusername/wg-route-panel.git`
2. Build and install: `make install`
3. Create admin: `sudo -u wgpanel /opt/wg-route-panel/bin/panel create-admin`
4. Start services: `sudo systemctl enable --now wg-route-panel xray`

See full documentation in the repository.
# wg-route-panel
