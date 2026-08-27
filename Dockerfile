# syntax=docker/dockerfile:1.7
FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/panel ./cmd/panel

FROM debian:bookworm-slim
ARG TARGETARCH
ARG SING_BOX_VERSION=1.13.19
ARG XRAY_VERSION=26.3.27
ARG SING_BOX_SHA256_AMD64=ef88a9e577d474210867bd708933d042e9b70106529df2656182c9db90106aa1
ARG SING_BOX_SHA256_ARM64=7fe3597a95a3c5ad67477b1d7653b9ce097e0be7c676758eba1fcf558f353d57
ARG XRAY_SHA256_AMD64=23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae
ARG XRAY_SHA256_ARM64=4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl iproute2 iputils-ping nftables sudo supervisor tini unzip \
    && rm -rf /var/lib/apt/lists/*
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) sb_arch=amd64; sb_sha="$SING_BOX_SHA256_AMD64"; xray_arch=64; xray_sha="$XRAY_SHA256_AMD64" ;; \
      arm64) sb_arch=arm64; sb_sha="$SING_BOX_SHA256_ARM64"; xray_arch=arm64-v8a; xray_sha="$XRAY_SHA256_ARM64" ;; \
      *) echo "Unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl --proto '=https' --tlsv1.2 -fsSL "https://github.com/SagerNet/sing-box/releases/download/v${SING_BOX_VERSION}/sing-box-${SING_BOX_VERSION}-linux-${sb_arch}.tar.gz" -o /tmp/sing-box.tar.gz; \
    echo "$sb_sha  /tmp/sing-box.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/sing-box.tar.gz -C /tmp; \
    install -m 0755 "/tmp/sing-box-${SING_BOX_VERSION}-linux-${sb_arch}/sing-box" /usr/local/bin/sing-box; \
    curl --proto '=https' --tlsv1.2 -fsSL "https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/Xray-linux-${xray_arch}.zip" -o /tmp/xray.zip; \
    echo "$xray_sha  /tmp/xray.zip" | sha256sum -c -; \
    unzip -q /tmp/xray.zip xray -d /tmp/xray; \
    install -m 0755 /tmp/xray/xray /usr/local/bin/xray; \
    rm -rf /tmp/sing-box* /tmp/xray*
RUN groupadd --system wgpanel \
    && useradd --system --gid wgpanel --home-dir /var/lib/wg-route-panel --shell /usr/sbin/nologin wgpanel \
    && printf 'wgpanel ALL=(root) NOPASSWD: /usr/local/sbin/wgpanel-nft-element *\n' >/etc/sudoers.d/wg-route-panel \
    && chmod 0440 /etc/sudoers.d/wg-route-panel
WORKDIR /app
COPY --from=builder /out/panel /usr/local/bin/wg-route-panel
COPY web ./web
COPY configs/panel.docker.yaml /etc/wg-route-panel/panel.yaml
COPY deploy/xray/base-config.json /usr/share/wg-route-panel/xray.json
COPY deploy/supervisor/wg-route-panel.conf /etc/supervisor/conf.d/wg-route-panel.conf
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY scripts/nft-element.sh /usr/local/sbin/wgpanel-nft-element
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh /usr/local/sbin/wgpanel-nft-element \
    && install -d -m 0750 -o wgpanel -g wgpanel /var/lib/wg-route-panel /etc/wg-route-panel
ENV PANEL_CONFIG_PATH=/etc/wg-route-panel/panel.yaml
EXPOSE 9090
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD curl -fsS http://127.0.0.1:9090/healthz || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
