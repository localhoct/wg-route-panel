package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
)

const validWGConfig = `[Interface]
Address = 10.0.0.2/32, fd00::2/128
PrivateKey = private-key
ListenPort = 51820
MTU = 1380

[Peer]
PublicKey = public-key
PresharedKey = preshared-key
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:51820
PersistentKeepalive = 25
`

type singBoxRunner struct {
	calls []call
	err   error
}

func (r *singBoxRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.calls = append(r.calls, call{name: name, args: args})
	return "", r.err
}

func newTestService(t *testing.T, runner *singBoxRunner) (*SingBoxService, *config.Config) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.WireGuard.ConfigPath = filepath.Join(dir, "wireguard", "wg0.conf")
	cfg.SingBox.ConfigPath = filepath.Join(dir, "sing-box", "config.json")
	cfg.SingBox.CacheFilePath = filepath.Join(dir, "sing-box", "cache.db")
	cfg.SingBox.BinaryPath = "/usr/local/bin/sing-box"
	db, err := repository.InitDB(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSingBoxService(cfg, db, runner), cfg
}

func TestParseWireGuardConfig(t *testing.T) {
	parsed, err := parseWireGuardConfig(validWGConfig)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.MTU != 1380 || parsed.ListenPort != 51820 || len(parsed.Addresses) != 2 || len(parsed.Peers) != 1 {
		t.Fatalf("unexpected parsed config: %#v", parsed)
	}
	if parsed.Peers[0].PersistentKeepalive != 25 || parsed.Peers[0].Address != "vpn.example.com" || parsed.Peers[0].Port != 51820 {
		t.Fatalf("unexpected peer: %#v", parsed.Peers[0])
	}
}

// TestGenerateSingBoxConfigUnifiesEverything verifies the single generated
// sing-box document contains the WireGuard endpoint (as a directly usable
// route outbound target, per the sing-box 1.13.19 behaviour confirmed via
// `sing-box check`), the SOCKS mixed inbound, the public DNS listener with
// sniff+hijack-dns routing, and the mandatory default_domain_resolver -
// covering the full replacement for the former split sing-box+Xray design.
func TestGenerateSingBoxConfigUnifiesEverything(t *testing.T) {
	runner := &singBoxRunner{}
	service, _ := newTestService(t, runner)

	ctx := context.Background()
	if err := repository.AddDNSRule(ctx, service.DB, "blocked.example", "block", ""); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddDNSRule(ctx, service.DB, "proxied.example", "proxy-route", ""); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddDNSRule(ctx, service.DB, "pinned.example", "static", "203.0.113.9"); err != nil {
		t.Fatal(err)
	}

	parsed, err := parseWireGuardConfig(validWGConfig)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := service.generateSingBoxConfig(ctx, parsed)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err = json.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}

	endpoints, ok := generated["endpoints"].([]any)
	if !ok || len(endpoints) != 1 {
		t.Fatalf("expected exactly one wireguard endpoint, got: %#v", generated["endpoints"])
	}
	endpoint := endpoints[0].(map[string]any)
	if endpoint["type"] != "wireguard" || endpoint["system"] != true || endpoint["tag"] != wireGuardEndpointTag {
		t.Fatalf("unexpected endpoint: %#v", endpoint)
	}

	inbounds := generated["inbounds"].([]any)
	foundSocks := false
	foundDNS := 0
	for _, raw := range inbounds {
		in := raw.(map[string]any)
		if in["type"] == "mixed" {
			foundSocks = true
		}
		if in["type"] == "direct" {
			foundDNS++
		}
	}
	if !foundSocks {
		t.Fatal("mixed SOCKS inbound missing")
	}
	if foundDNS != 2 {
		t.Fatalf("expected 2 direct DNS inbounds (udp+tcp), got %d", foundDNS)
	}

	route := generated["route"].(map[string]any)
	if route["default_domain_resolver"] != localResolverTag {
		t.Fatalf("default_domain_resolver must be set (sing-box 1.12+ requires it): %#v", route)
	}
	rules := route["rules"].([]any)
	if len(rules) < 2 {
		t.Fatalf("expected sniff+hijack-dns rules at minimum: %#v", rules)
	}
	firstRule := rules[0].(map[string]any)
	if firstRule["action"] != "sniff" {
		t.Fatalf("first route rule must sniff DNS inbounds: %#v", firstRule)
	}
	secondRule := rules[1].(map[string]any)
	if secondRule["action"] != "hijack-dns" {
		t.Fatalf("second route rule must hijack DNS traffic: %#v", secondRule)
	}

	// The wireguard endpoint tag must be directly usable as a route
	// outbound - the whole point of not needing an Xray-style
	// interface-binding trick.
	foundProxyRoute := false
	for _, raw := range rules {
		r := raw.(map[string]any)
		if r["outbound"] == wireGuardEndpointTag {
			foundProxyRoute = true
		}
	}
	if !foundProxyRoute {
		t.Fatalf("expected a route rule targeting the wireguard endpoint directly: %#v", rules)
	}

	dns := generated["dns"].(map[string]any)
	if dns["servers"] == nil {
		t.Fatal("dns.servers missing")
	}
}

// TestGenerateSingBoxConfigWithoutPeers ensures the generator degrades
// gracefully (no endpoint, no proxy-route DNS server) when the operator has
// not yet uploaded a WireGuard configuration, instead of crashing.
func TestGenerateSingBoxConfigWithoutPeers(t *testing.T) {
	runner := &singBoxRunner{}
	service, _ := newTestService(t, runner)
	raw, err := service.generateSingBoxConfig(context.Background(), wireGuardConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err = json.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	if _, present := generated["endpoints"]; present {
		t.Fatalf("no endpoints should be generated without peers: %#v", generated["endpoints"])
	}
}

func TestSaveConfigPersistsAndValidates(t *testing.T) {
	runner := &singBoxRunner{}
	service, cfg := newTestService(t, runner)
	if err := service.SaveConfig(context.Background(), validWGConfig); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != cfg.SingBox.BinaryPath || runner.calls[0].args[0] != "check" {
		t.Fatalf("expected exactly one sing-box check call, got: %#v", runner.calls)
	}
	for _, path := range []string{cfg.WireGuard.ConfigPath, cfg.SingBox.ConfigPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s mode = %o, want 0600", path, info.Mode().Perm())
		}
	}
}

func TestSingBoxSupervisorLifecycle(t *testing.T) {
	runner := &singBoxRunner{}
	service, cfg := newTestService(t, runner)
	if err := service.Action(context.Background(), "restart"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != cfg.SingBox.SupervisorCtl || runner.calls[0].args[0] != "restart" || runner.calls[0].args[1] != "sing-box" {
		t.Fatalf("unexpected supervisor call: %#v", runner.calls)
	}
}

func TestActionRejectsInvalidVerb(t *testing.T) {
	runner := &singBoxRunner{}
	service, _ := newTestService(t, runner)
	if err := service.Action(context.Background(), "explode"); err == nil {
		t.Fatal("expected invalid action to be rejected")
	}
}
