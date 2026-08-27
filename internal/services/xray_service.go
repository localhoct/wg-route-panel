package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
)

type XrayService struct {
	DB     *sql.DB
	Config *config.Config
	Runner system.Runner
}

func (x *XrayService) Generate(ctx context.Context) error {
	rules, e := repository.DNSRules(ctx, x.DB)
	if e != nil {
		return e
	}
	cats, e := repository.GeositeCategories(ctx, x.DB, "")
	if e != nil {
		return e
	}
	routing := []any{}
	hosts := map[string]string{}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		switch r.Action {
		case "block":
			routing = append(routing, map[string]any{"type": "field", "domain": []string{"domain:" + r.Domain}, "outboundTag": "block"})
		case "proxy-route":
			routing = append(routing, map[string]any{"type": "field", "domain": []string{"domain:" + r.Domain}, "outboundTag": "wg-out"})
		case "static":
			hosts[r.Domain] = r.StaticIP
		}
	}
	for _, c := range cats {
		if c.Selected {
			tag := "direct"
			if c.Action == "block" {
				tag = "block"
			} else if c.Action == "proxy-route" {
				tag = "wg-out"
			}
			routing = append(routing, map[string]any{"type": "field", "domain": []string{"geosite:" + c.Tag}, "outboundTag": tag})
		}
	}
	socksSettings := map[string]any{"auth": "noauth", "udp": true}
	if x.Config.SOCKS.Username != "" && x.Config.SOCKS.Password != "" {
		socksSettings["auth"] = "password"
		socksSettings["accounts"] = []any{map[string]string{"user": x.Config.SOCKS.Username, "pass": x.Config.SOCKS.Password}}
	}
	cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "dns": map[string]any{"hosts": hosts, "servers": []any{x.Config.DNS.DirectUpstream, x.Config.DNS.TunnelUpstream}}, "inbounds": []any{map[string]any{"tag": "socks-in", "listen": x.Config.SOCKS.ListenAddr, "port": x.Config.SOCKS.Port, "protocol": "socks", "settings": socksSettings}, map[string]any{"tag": "dns-in", "listen": x.Config.DNS.ListenAddr, "port": x.Config.DNS.Port, "protocol": "dokodemo-door", "settings": map[string]any{"address": x.Config.DNS.TunnelUpstream, "port": 53, "network": "tcp,udp"}}}, "outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}, map[string]any{"tag": "wg-out", "protocol": "freedom", "streamSettings": map[string]any{"sockopt": map[string]any{"interface": x.Config.WireGuard.InterfaceName}}}, map[string]any{"tag": "block", "protocol": "blackhole"}}, "routing": map[string]any{"domainStrategy": "IPIfNonMatch", "rules": routing}}
	b, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		return e
	}
	if e = system.AtomicWrite(x.Config.Xray.ConfigPath, b, 0640); e != nil {
		return e
	}
	if _, e = x.Runner.Run(ctx, x.Config.Xray.BinaryPath, "run", "-test", "-config", x.Config.Xray.ConfigPath); e != nil {
		return fmt.Errorf("xray config validation: %w", e)
	}
	return x.control(ctx, "restart")
}
func (x *XrayService) Action(ctx context.Context, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("invalid action")
	}
	return x.control(ctx, action)
}

func (x *XrayService) control(ctx context.Context, action string) error {
	if x.Config.Xray.Manager == "systemd" {
		_, err := x.Runner.Run(ctx, "sudo", "/usr/bin/systemctl", action, x.Config.Xray.ServiceName)
		return err
	}
	_, err := x.Runner.Run(ctx, x.Config.Xray.SupervisorCtl, action, x.Config.Xray.ServiceName)
	return err
}
