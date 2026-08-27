package services

import (
	"bufio"
	"context"
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
	"github.com/localhoct/wg-route-panel/internal/system"
)

type WireGuardService struct {
	Interface     string
	ConfigPath    string
	SingBoxPath   string
	SingBoxBinary string
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

func NewWireGuardService(cfg *config.Config, runner system.Runner) *WireGuardService {
	return &WireGuardService{
		Interface:     cfg.WireGuard.InterfaceName,
		ConfigPath:    cfg.WireGuard.ConfigPath,
		SingBoxPath:   cfg.SingBox.ConfigPath,
		SingBoxBinary: cfg.SingBox.BinaryPath,
		Manager:       cfg.SingBox.Manager,
		SupervisorCtl: cfg.SingBox.SupervisorCtl,
		ProbeAddress:  cfg.WireGuard.ProbeAddress,
		Runner:        runner,
	}
}

func (s *WireGuardService) ValidateConfig(raw string) error {
	_, err := parseWireGuardConfig(raw)
	return err
}

func (s *WireGuardService) SaveConfig(raw string) error {
	parsed, err := parseWireGuardConfig(raw)
	if err != nil {
		return err
	}
	generated, err := s.generateSingBoxConfig(parsed)
	if err != nil {
		return err
	}
	candidate, err := writeCandidate(s.SingBoxPath, generated)
	if err != nil {
		return err
	}
	defer os.Remove(candidate)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = s.Runner.Run(ctx, s.SingBoxBinary, "check", "-c", candidate); err != nil {
		return fmt.Errorf("sing-box configuration validation failed: %w", err)
	}
	if err = os.Rename(candidate, s.SingBoxPath); err != nil {
		return fmt.Errorf("activate sing-box configuration: %w", err)
	}
	if err = system.AtomicWrite(s.ConfigPath, []byte(raw), 0600); err != nil {
		return err
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

func (s *WireGuardService) Config() string {
	b, _ := os.ReadFile(s.ConfigPath)
	return string(b)
}

func (s *WireGuardService) Action(ctx context.Context, action string) error {
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

func (s *WireGuardService) GetStatus(ctx context.Context) models.WireGuardStatus {
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
	if raw, readErr := os.ReadFile(s.ConfigPath); readErr == nil {
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

func (s *WireGuardService) generateSingBoxConfig(cfg wireGuardConfig) ([]byte, error) {
	peers := make([]map[string]any, 0, len(cfg.Peers))
	needsResolver := false
	for _, peer := range cfg.Peers {
		if net.ParseIP(peer.Address) == nil {
			needsResolver = true
		}
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
	endpoint := map[string]any{
		"type":        "wireguard",
		"tag":         "wg-ep",
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
	route := map[string]any{"auto_detect_interface": true}
	output := map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true},
		"endpoints": []any{endpoint},
		"route":     route,
	}
	if needsResolver {
		output["dns"] = map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}}
		route["default_domain_resolver"] = "local"
	}
	return json.MarshalIndent(output, "", "  ")
}
