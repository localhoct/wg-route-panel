#!/bin/bash
set -e

echo "🚀 Installing WG Route Panel..."

# Check Ubuntu version
if ! grep -q "Ubuntu" /etc/os-release; then
    echo "❌ This script is designed for Ubuntu."
    exit 1
fi

# Install dependencies
echo "📦 Installing system dependencies..."
apt-get update
apt-get install -y wireguard wireguard-tools nftables curl ca-certificates

# Create system user
if ! id "wgpanel" &>/dev/null; then
    useradd -r -s /bin/false wgpanel
fi

# Create directories
mkdir -p /etc/wg-route-panel/{wireguard,xray,geosite}
mkdir -p /var/lib/wg-route-panel
mkdir -p /var/log/wg-route-panel

chown -R wgpanel:wgpanel /etc/wg-route-panel /var/lib/wg-route-panel /var/log/wg-route-panel
chmod 750 /etc/wg-route-panel
chmod 600 /etc/wg-route-panel/wireguard/wg0.conf 2>/dev/null || true

# Copy binary (assuming it's in the current directory or built)
if [ -f ./bin/panel ]; then
    mkdir -p /opt/wg-route-panel/bin
    cp ./bin/panel /opt/wg-route-panel/bin/
    chown root:wgpanel /opt/wg-route-panel/bin/panel
    chmod 750 /opt/wg-route-panel/bin/panel
fi

# Install systemd services
cp deploy/systemd/panel.service /etc/systemd/system/
cp deploy/systemd/xray.service /etc/systemd/system/
systemctl daemon-reload

# Setup nftables
if [ -f deploy/nftables/wg-route-panel.nft ]; then
    nft -f deploy/nftables/wg-route-panel.nft
fi

# Setup sudoers
cp deploy/sudoers/wg-route-panel.sudoers /etc/sudoers.d/wg-route-panel
chmod 440 /etc/sudoers.d/wg-route-panel

echo "✅ Installation complete!"
echo "Next steps:"
echo "1. Edit /etc/wg-route-panel/panel.yaml"
echo "2. Run: sudo -u wgpanel /opt/wg-route-panel/bin/panel create-admin"
echo "3. Run: sudo systemctl enable --now wg-route-panel"
