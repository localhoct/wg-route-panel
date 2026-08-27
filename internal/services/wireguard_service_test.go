package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/system"
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

type wireGuardRunner struct {
	calls []call
	err   error
}

func (r *wireGuardRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.calls = append(r.calls, call{name: name, args: args})
	return "", r.err
}

func TestParseAndGenerateSingBoxWireGuardEndpoint(t *testing.T) {
	parsed, err := parseWireGuardConfig(validWGConfig)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.MTU != 1380 || parsed.ListenPort != 51820 || len(parsed.Addresses) != 2 || len(parsed.Peers) != 1 {
		t.Fatalf("unexpected parsed config: %#v", parsed)
	}

	service := &WireGuardService{Interface: "wg0"}
	raw, err := service.generateSingBoxConfig(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err = json.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	endpoints := generated["endpoints"].([]any)
	endpoint := endpoints[0].(map[string]any)
	if endpoint["type"] != "wireguard" || endpoint["system"] != true || endpoint["name"] != "wg0" {
		t.Fatalf("unexpected endpoint: %#v", endpoint)
	}
	route := generated["route"].(map[string]any)
	if route["default_domain_resolver"] != "local" || generated["dns"] == nil {
		t.Fatalf("hostname resolver is missing: %#v", generated)
	}
}

func TestSaveConfigValidatesGeneratedSingBoxConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.WireGuard.ConfigPath = filepath.Join(dir, "wireguard", "wg0.conf")
	cfg.SingBox.ConfigPath = filepath.Join(dir, "sing-box", "config.json")
	cfg.SingBox.BinaryPath = "/usr/local/bin/sing-box"
	runner := &wireGuardRunner{}
	service := NewWireGuardService(cfg, runner)

	if err := service.SaveConfig(validWGConfig); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != cfg.SingBox.BinaryPath || len(runner.calls[0].args) != 3 || runner.calls[0].args[0] != "check" || runner.calls[0].args[1] != "-c" || filepath.Dir(runner.calls[0].args[2]) != filepath.Dir(cfg.SingBox.ConfigPath) {
		t.Fatalf("unexpected validation call: %#v", runner.calls)
	}
	for _, path := range []string{cfg.WireGuard.ConfigPath, cfg.SingBox.ConfigPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}
}

func TestWireGuardSupervisorLifecycle(t *testing.T) {
	cfg := config.Default()
	runner := &wireGuardRunner{}
	service := NewWireGuardService(cfg, runner)
	if err := service.Action(context.Background(), "restart"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "/usr/bin/supervisorctl" || strings.Join(runner.calls[0].args, " ") != "restart sing-box" {
		t.Fatalf("unexpected lifecycle call: %#v", runner.calls)
	}
	if err := service.Action(context.Background(), "reload"); err == nil {
		t.Fatal("invalid lifecycle action accepted")
	}
}

func TestWireGuardConfigurationValidation(t *testing.T) {
	tests := []struct {
		name       string
		replaceOld string
		replaceNew string
	}{
		{name: "endpoint required", replaceOld: "Endpoint = vpn.example.com:51820", replaceNew: "# no endpoint"},
		{name: "invalid address", replaceOld: "10.0.0.2/32", replaceNew: "10.0.0.999/32"},
		{name: "invalid port", replaceOld: "vpn.example.com:51820", replaceNew: "vpn.example.com:70000"},
		{name: "invalid mtu", replaceOld: "MTU = 1380", replaceNew: "MTU = 100"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseWireGuardConfig(strings.Replace(validWGConfig, test.replaceOld, test.replaceNew, 1))
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestGeneratedConfigWithSingBox(t *testing.T) {
	binary := os.Getenv("SING_BOX_BINARY")
	if binary == "" {
		t.Skip("set SING_BOX_BINARY to run the sing-box integration check")
	}
	output, err := system.ExecRunner{}.Run(context.Background(), binary, "generate", "wg-keypair")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(output)
	if len(fields) != 4 || fields[0] != "PrivateKey:" || fields[2] != "PublicKey:" {
		t.Fatalf("unexpected keypair output: %q", output)
	}
	raw := fmt.Sprintf(`[Interface]
Address = 10.0.0.2/32
PrivateKey = %s

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0
Endpoint = vpn.example.com:51820
PersistentKeepalive = 25
`, fields[1], fields[3])
	dir := t.TempDir()
	cfg := config.Default()
	cfg.WireGuard.ConfigPath = filepath.Join(dir, "wg0.conf")
	cfg.SingBox.ConfigPath = filepath.Join(dir, "config.json")
	cfg.SingBox.BinaryPath = binary
	if err = NewWireGuardService(cfg, system.ExecRunner{}).SaveConfig(raw); err != nil {
		t.Fatal(err)
	}
}

func TestSaveConfigReportsSingBoxValidationFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.WireGuard.ConfigPath = filepath.Join(dir, "wg0.conf")
	cfg.SingBox.ConfigPath = filepath.Join(dir, "config.json")
	runner := &wireGuardRunner{err: errors.New("invalid config")}
	if err := NewWireGuardService(cfg, runner).SaveConfig(validWGConfig); err == nil || !strings.Contains(err.Error(), "sing-box configuration validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
