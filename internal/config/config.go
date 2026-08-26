package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2" // Using toml as a stand-in for yaml for simplicity, or use gopkg.in/yaml.v3
)

type Config struct {
	ListenAddr   string `toml:"listen_addr"`
	SessionSecret string `toml:"session_secret"`
	DBPath       string `toml:"db_path"`
	WireGuard    WGConfig `toml:"wireguard"`
	Xray         XrayConfig `toml:"xray"`
	 Firewall    FWConfig `toml:"firewall"`
}

type WGConfig struct {
	InterfaceName string `toml:"interface_name"`
	ConfigPath    string `toml:"config_path"`
}

type XrayConfig struct {
	BinaryPath  string `toml:"binary_path"`
	ConfigPath  string `toml:"config_path"`
}

type FWConfig struct {
	Enabled bool `toml:"enabled"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Return default config for development if file doesn't exist
		return defaultConfig(), nil
	}
	
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	
	// Override with env vars if present
	if secret := os.Getenv("PANEL_SESSION_SECRET"); secret != "" {
		cfg.SessionSecret = secret
	}
	if db := os.Getenv("PANEL_DB_PATH"); db != "" {
		cfg.DBPath = db
	}
	
	return &cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		ListenAddr:    "127.0.0.1:8080",
		SessionSecret: "change-me-in-production",
		DBPath:        "/var/lib/wg-route-panel/panel.db",
		WireGuard:     WGConfig{InterfaceName: "wg0", ConfigPath: "/etc/wg-route-panel/wireguard/wg0.conf"},
		Xray:          XrayConfig{BinaryPath: "/usr/local/bin/xray", ConfigPath: "/etc/wg-route-panel/xray/config.json"},
		Firewall:      FWConfig{Enabled: true},
	}
}
