package services

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/models"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
)

// SingBoxService owns the entire sing-box lifecycle for this panel: it
// converts the operator-supplied WireGuard configuration into a
// sing-box "wireguard" endpoint, compiles the DNS rules, geosite/rule-set
// selections and SOCKS settings stored in the database into a single
// unified sing-box configuration (inbounds + dns + route), validates it
// with the real `sing-box check` binary before activating it, and drives
// the supervisor/systemd-managed sing-box process.
//
// This replaces the former split design where sing-box only handled the
// WireGuard tunnel and a separate Xray-core process handled SOCKS5, DNS
// interception and geosite-based routing. sing-box's WireGuard endpoint
// tag can be referenced directly as a route.rules[].outbound target, so
// no interface-binding trick (Xray's sockopt.interface) is required to
// route arbitrary traffic through the tunnel.
type SingBoxService struct {
	DB *sql.DB

	Interface     string
	WGConfigPath  string
	SingBoxPath   string
	SingBoxBinary string
	CacheFilePath string
	Manager       string
	SupervisorCtl string
	ProbeAddress  string
	Runner        system.Runner
}

type wireGuardPeer struct {
	Address             string
	Port                uint16
	PublicKey           string
	PreSharedKey        string
	AllowedIPs          []string
	PersistentKeepalive int
}

type wireGuardConfig struct {
	Addresses  []string
	PrivateKey string
	ListenPort uint16
	MTU        int
	Peers      []wireGuardPeer
}

func NewSingBoxService(cfg *config.Config, db *sql.DB, runner system.Runner) *SingBoxService {
	return &SingBoxService{
		DB:            db,
		Interface:     cfg.WireGuard.InterfaceName,
		WGConfigPath:  cfg.WireGuard.ConfigPath,
		SingBoxPath:   cfg.SingBox.ConfigPath,
		SingBoxBinary: cfg.SingBox.BinaryPath,
		CacheFilePath: cfg.SingBox.CacheFilePath,
		Manager:       cfg.SingBox.Manager,
		SupervisorCtl: cfg.SingBox.SupervisorCtl,
		ProbeAddress:  cfg.WireGuard.ProbeAddress,
		Runner:        runner,
	}
}

func (s *SingBoxService) ValidateConfig(raw string) error {
	_, err := parseWireGuardConfig(raw)
	return err
}

func (s *SingBoxService) Config() string {
	b, _ := os.ReadFile(s.WGConfigPath)
	return string(b)
}

// SaveConfig parses and persists a new WireGuard source configuration,
// then regenerates and activates the unified sing-box configuration.
func (s *SingBoxService) SaveConfig(ctx context.Context, raw string) error {
	parsed, err := parseWireGuardConfig(raw)
	if err != nil {
		return err
	}
	if err = system.AtomicWrite(s.WGConfigPath, []byte(raw), 0600); err != nil {
		return err
	}
	return s.Regenerate(ctx, &parsed)
}

// Regenerate rebuilds the full sing-box configuration from the current
// WireGuard configuration on disk plus the DNS rules, geosite selections
// and SOCKS settings stored in the database, validates it with
// `sing-box check`, and atomically activates it. It is called after every
// change made from the web panel (DNS rule add/delete, geosite/rule-set
// assignment, SOCKS settings update) so operators never need to touch
// the server to apply a change.
func (s *SingBoxService) Regenerate(ctx context.Context, wg *wireGuardConfig) error {
	if wg == nil {
		raw, err := os.ReadFile(s.WGConfigPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				empty := wireGuardConfig{}
				wg = &empty
			} else {
				return err
			}
		} else {
			parsed, err := parseWireGuardConfig(string(raw))
			if err != nil {
				return err
			}
			wg = &parsed
		}
	}
	generated, err := s.generateSingBoxConfig(ctx, *wg)
	if err != nil {
		return err
	}
	candidate, err := writeCandidate(s.SingBoxPath, generated)
	if err != nil {
		return err
	}
	defer os.Remove(candidate)
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err = s.Runner.Run(checkCtx, s.SingBoxBinary, "check", "-c", candidate); err != nil {
		return fmt.Errorf("sing-box configuration validation failed: %w", err)
	}
	if err = os.Rename(candidate, s.SingBoxPath); err != nil {
		return fmt.Errorf("activate sing-box configuration: %w", err)
	}
	return nil
}

func writeCandidate(target string, data []byte) (string, error) {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".sing-box-candidate-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if err = file.Chmod(0600); err != nil {
		cleanup()
		return "", err
	}
	if _, err = file.Write(data); err != nil {
		cleanup()
		return "", err
	}
	if err = file.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (s *SingBoxService) Action(ctx context.Context, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return errors.New("invalid action")
	}
	if s.Manager == "systemd" {
		_, err := s.Runner.Run(ctx, "sudo", "/usr/bin/systemctl", action, "sing-box.service")
		return err
	}
	if s.SupervisorCtl == "" {
		s.SupervisorCtl = "/usr/bin/supervisorctl"
	}
	_, err := s.Runner.Run(ctx, s.SupervisorCtl, action, "sing-box")
	return err
}

func (s *SingBoxService) GetStatus(ctx context.Context) models.WireGuardStatus {
	status := models.WireGuardStatus{State: "stopped", CheckedAt: time.Now()}
	var output string
	var err error
	if s.Manager == "systemd" {
		output, err = s.Runner.Run(ctx, "sudo", "/usr/bin/systemctl", "is-active", "sing-box.service")
		if err != nil || strings.TrimSpace(output) != "active" {
			if err != nil {
				status.LastError = err.Error()
			}
			return status
		}
	} else {
		output, err = s.Runner.Run(ctx, s.SupervisorCtl, "status", "sing-box")
		if err != nil || !strings.Contains(output, "RUNNING") {
			if err != nil {
				status.LastError = err.Error()
			} else {
				status.LastError = strings.TrimSpace(output)
			}
			return status
		}
	}
	status.State = "degraded"
	if _, err = s.Runner.Run(ctx, "/sbin/ip", "link", "show", "dev", s.Interface); err != nil {
		status.LastError = "sing-box is running but the WireGuard interface is unavailable"
		return status
	}
	if raw, readErr := os.ReadFile(s.WGConfigPath); readErr == nil {
		if parsed, parseErr := parseWireGuardConfig(string(raw)); parseErr == nil && len(parsed.Peers) > 0 {
			status.Endpoint = net.JoinHostPort(parsed.Peers[0].Address, strconv.Itoa(int(parsed.Peers[0].Port)))
		}
	}
	status.RXBytes = readCounter("/sys/class/net/" + s.Interface + "/statistics/rx_bytes")
	status.TXBytes = readCounter("/sys/class/net/" + s.Interface + "/statistics/tx_bytes")
	if s.ProbeAddress != "" {
		if _, err = s.Runner.Run(ctx, "/bin/ping", "-I", s.Interface, "-c", "1", "-W", "2", s.ProbeAddress); err != nil {
			status.LastError = "interface is up but the configured probe failed"
			return status
		}
	}
	status.State = "connected"
	status.LatestHandshake = "managed internally by sing-box"
	return status
}

func readCounter(path string) int64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return value
}

func parseWireGuardConfig(raw string) (wireGuardConfig, error) {
	if err := system.ValidateWGConfig(raw); err != nil {
		return wireGuardConfig{}, err
	}
	var cfg wireGuardConfig
	cfg.MTU = 1408
	section := ""
	peer := -1
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			if section == "Peer" {
				cfg.Peers = append(cfg.Peers, wireGuardPeer{})
				peer = len(cfg.Peers) - 1
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if section == "Interface" {
			switch key {
			case "Address":
				cfg.Addresses = splitCSV(value)
			case "PrivateKey":
				cfg.PrivateKey = value
			case "ListenPort":
				port, err := parsePort(value)
				if err != nil {
					return cfg, fmt.Errorf("ListenPort: %w", err)
				}
				cfg.ListenPort = port
			case "MTU":
				mtu, err := strconv.Atoi(value)
				if err != nil || mtu < 576 || mtu > 9000 {
					return cfg, errors.New("MTU must be between 576 and 9000")
				}
				cfg.MTU = mtu
			}
		} else if section == "Peer" && peer >= 0 {
			switch key {
			case "PublicKey":
				cfg.Peers[peer].PublicKey = value
			case "PresharedKey":
				cfg.Peers[peer].PreSharedKey = value
			case "AllowedIPs":
				cfg.Peers[peer].AllowedIPs = splitCSV(value)
			case "Endpoint":
				host, port, err := net.SplitHostPort(value)
				if err != nil {
					return cfg, fmt.Errorf("peer %d Endpoint: %w", peer+1, err)
				}
				parsedPort, err := parsePort(port)
				if err != nil {
					return cfg, fmt.Errorf("peer %d Endpoint: %w", peer+1, err)
				}
				cfg.Peers[peer].Address, cfg.Peers[peer].Port = host, parsedPort
			case "PersistentKeepalive":
				keepalive, err := strconv.Atoi(value)
				if err != nil || keepalive < 0 || keepalive > 65535 {
					return cfg, fmt.Errorf("peer %d has invalid PersistentKeepalive", peer+1)
				}
				cfg.Peers[peer].PersistentKeepalive = keepalive
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	for index, candidate := range cfg.Addresses {
		if _, _, err := net.ParseCIDR(candidate); err != nil {
			return cfg, fmt.Errorf("Address %d is invalid: %w", index+1, err)
		}
	}
	for index, candidate := range cfg.Peers {
		if candidate.Address == "" || candidate.Port == 0 {
			return cfg, fmt.Errorf("peer %d requires Endpoint", index+1)
		}
		for _, prefix := range candidate.AllowedIPs {
			if _, _, err := net.ParseCIDR(prefix); err != nil {
				return cfg, fmt.Errorf("peer %d has invalid AllowedIPs entry: %w", index+1, err)
			}
		}
	}
	return cfg, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parsePort(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	return uint16(port), nil
}

const wireGuardEndpointTag = "wg-ep"
const localResolverTag = "local-resolver"
const directDNSTag = "direct-dns"
const proxyDNSTag = "proxy-dns"
const staticHostsTag = "static-hosts"
const directOutboundTag = "direct-out"
const blockOutboundTag = "block-out"
const socksInboundTag = "socks-in"
const dnsInboundUDPTag = "dns-in-udp"
const dnsInboundTCPTag = "dns-in-tcp"

// generateSingBoxConfig compiles the WireGuard tunnel, SOCKS inbound, DNS
// interception/routing and geosite rule-sets into one sing-box
// configuration document. See the package doc comment on SingBoxService
// for the overall design rationale.
func (s *SingBoxService) generateSingBoxConfig(ctx context.Context, cfg wireGuardConfig) ([]byte, error) {
	hasPeers := len(cfg.Peers) > 0

	peers := make([]map[string]any, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		entry := map[string]any{
			"address":                       peer.Address,
			"port":                          peer.Port,
			"public_key":                    peer.PublicKey,
			"allowed_ips":                   peer.AllowedIPs,
			"persistent_keepalive_interval": peer.PersistentKeepalive,
		}
		if peer.PreSharedKey != "" {
			entry["pre_shared_key"] = peer.PreSharedKey
		}
		peers = append(peers, entry)
	}

	var endpoints []any
	if hasPeers {
		endpoint := map[string]any{
			"type":        "wireguard",
			"tag":         wireGuardEndpointTag,
			"system":      true,
			"name":        s.Interface,
			"mtu":         cfg.MTU,
			"address":     cfg.Addresses,
			"private_key": cfg.PrivateKey,
			"peers":       peers,
		}
		if cfg.ListenPort != 0 {
			endpoint["listen_port"] = cfg.ListenPort
		}
		endpoints = append(endpoints, endpoint)
	}

	dnsRules, err := repository.DNSRules(ctx, s.DB)
	if err != nil {
		return nil, fmt.Errorf("load dns rules: %w", err)
	}
	categories, err := repository.SelectedGeositeCategories(ctx, s.DB)
	if err != nil {
		return nil, fmt.Errorf("load geosite categories: %w", err)
	}
	dnsSettings, err := repository.DNSSettings(ctx, s.DB)
	if err != nil {
		return nil, fmt.Errorf("load dns settings: %w", err)
	}
	socksSettings, err := repository.SocksSettings(ctx, s.DB)
	if err != nil {
		return nil, fmt.Errorf("load socks settings: %w", err)
	}

	// ---- DNS module -----------------------------------------------
	dnsServers := []any{map[string]any{"type": "local", "tag": localResolverTag}}
	dnsServers = append(dnsServers, map[string]any{
		"type":   "udp",
		"tag":    directDNSTag,
		"server": dnsSettings.DirectUpstream,
	})
	if hasPeers {
		dnsServers = append(dnsServers, map[string]any{
			"type":   "udp",
			"tag":    proxyDNSTag,
			"server": dnsSettings.ProxyUpstream,
			"detour": wireGuardEndpointTag,
		})
	}
	staticHosts := map[string]any{}
	for _, r := range dnsRules {
		if r.Enabled && r.Action == "static" && r.StaticIP != "" {
			staticHosts[r.Domain] = r.StaticIP
		}
	}
	dnsServers = append(dnsServers, map[string]any{
		"type":       "hosts",
		"tag":        staticHostsTag,
		"predefined": staticHosts,
	})

	var dnsRuleEntries []any
	for _, r := range dnsRules {
		if !r.Enabled {
			continue
		}
		switch r.Action {
		case "static":
			dnsRuleEntries = append(dnsRuleEntries, map[string]any{"domain": r.Domain, "server": staticHostsTag})
		case "block":
			dnsRuleEntries = append(dnsRuleEntries, map[string]any{"domain": r.Domain, "action": "reject"})
		case "proxy-route":
			if hasPeers {
				dnsRuleEntries = append(dnsRuleEntries, map[string]any{"domain": r.Domain, "server": proxyDNSTag})
			}
		case "direct":
			dnsRuleEntries = append(dnsRuleEntries, map[string]any{"domain": r.Domain, "server": directDNSTag})
		}
	}
	for _, c := range categories {
		ruleSetTag := "geosite-" + c.Tag
		switch c.Action {
		case "block":
			dnsRuleEntries = append(dnsRuleEntries, map[string]any{"rule_set": ruleSetTag, "action": "reject"})
		case "proxy-route":
			if hasPeers {
				dnsRuleEntries = append(dnsRuleEntries, map[string]any{"rule_set": ruleSetTag, "server": proxyDNSTag})
			}
		default:
			dnsRuleEntries = append(dnsRuleEntries, map[string]any{"rule_set": ruleSetTag, "server": directDNSTag})
		}
	}

	dns := map[string]any{
		"servers": dnsServers,
		"final":   directDNSTag,
	}
	if len(dnsRuleEntries) > 0 {
		dns["rules"] = dnsRuleEntries
	}

	// ---- Inbounds ---------------------------------------------------
	inbounds := []any{
		map[string]any{
			"type":        "direct",
			"tag":         dnsInboundUDPTag,
			"listen":      dnsSettings.ListenAddr,
			"listen_port": dnsSettings.Port,
			"network":     "udp",
		},
		map[string]any{
			"type":        "direct",
			"tag":         dnsInboundTCPTag,
			"listen":      dnsSettings.ListenAddr,
			"listen_port": dnsSettings.Port,
			"network":     "tcp",
		},
	}
	socksInbound := map[string]any{
		"type":        "mixed",
		"tag":         socksInboundTag,
		"listen":      socksSettings.ListenAddr,
		"listen_port": socksSettings.Port,
	}
	if socksSettings.Username != "" && socksSettings.Password != "" {
		socksInbound["users"] = []any{map[string]any{"username": socksSettings.Username, "password": socksSettings.Password}}
	}
	inbounds = append(inbounds, socksInbound)

	// ---- Outbounds ---------------------------------------------------
	outbounds := []any{
		map[string]any{"type": "direct", "tag": directOutboundTag},
		map[string]any{"type": "block", "tag": blockOutboundTag},
	}

	// ---- Route rule-sets + rules --------------------------------------
	var ruleSets []any
	var routeRules []any
	routeRules = append(routeRules, map[string]any{
		"inbound": []any{dnsInboundUDPTag, dnsInboundTCPTag},
		"action":  "sniff",
	})
	routeRules = append(routeRules, map[string]any{
		"protocol": "dns",
		"action":   "hijack-dns",
	})
	for _, r := range dnsRules {
		if !r.Enabled {
			continue
		}
		switch r.Action {
		case "block":
			routeRules = append(routeRules, map[string]any{"domain": r.Domain, "action": "reject"})
		case "proxy-route":
			if hasPeers {
				routeRules = append(routeRules, map[string]any{"domain": r.Domain, "action": "route", "outbound": wireGuardEndpointTag})
			}
		case "direct", "static":
			routeRules = append(routeRules, map[string]any{"domain": r.Domain, "action": "route", "outbound": directOutboundTag})
		}
	}
	for _, c := range categories {
		ruleSetTag := "geosite-" + c.Tag
		ruleSets = append(ruleSets, map[string]any{
			"type":   "remote",
			"tag":    ruleSetTag,
			"format": "binary",
			"url":    RuleSetURL(c.Tag),
		})
		switch c.Action {
		case "block":
			routeRules = append(routeRules, map[string]any{"rule_set": ruleSetTag, "action": "reject"})
		case "proxy-route":
			if hasPeers {
				routeRules = append(routeRules, map[string]any{"rule_set": ruleSetTag, "action": "route", "outbound": wireGuardEndpointTag})
			}
		default:
			routeRules = append(routeRules, map[string]any{"rule_set": ruleSetTag, "action": "route", "outbound": directOutboundTag})
		}
	}
	routeRules = append(routeRules, map[string]any{"ip_is_private": true, "outbound": directOutboundTag})

	route := map[string]any{
		"rules":                   routeRules,
		"final":                   directOutboundTag,
		"auto_detect_interface":   true,
		"default_domain_resolver": localResolverTag,
	}
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}

	output := map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true},
		"dns":       dns,
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}
	if hasPeers {
		output["endpoints"] = endpoints
	}
	if s.CacheFilePath != "" {
		output["experimental"] = map[string]any{
			"cache_file": map[string]any{"enabled": true, "path": s.CacheFilePath},
		}
	}
	return json.MarshalIndent(output, "", "  ")
}
