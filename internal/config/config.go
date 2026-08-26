package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr    string       `yaml:"listen_addr"`
	BaseURL       string       `yaml:"base_url"`
	SessionSecret string       `yaml:"session_secret"`
	DBPath        string       `yaml:"db_path"`
	WireGuard     WGConfig     `yaml:"wireguard"`
	Xray          XrayConfig   `yaml:"xray"`
	Geosite       GeositeConfig`yaml:"geosite"`
	DNS           DNSConfig    `yaml:"dns"`
	SOCKS         SOCKSConfig  `yaml:"socks"`
	Firewall      FWConfig     `yaml:"firewall"`
	Log           LogConfig    `yaml:"log"`
}

type WGConfig struct {
	InterfaceName string `yaml:"interface_name"`
	ConfigPath    string `yaml:"config_path"`
}

type XrayConfig struct {
	BinaryPath string `yaml:"binary_path"`
	ConfigPath string `yaml:"config_path"`
}

type GeositeConfig struct {
	Path      string `yaml:"path"`
	UpdateURL string `yaml:"update_url"`
}

type DNSConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	Port       int    `yaml:"port"`
}

type SOCKSConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	Port       int    `yaml:"port"`
}

type FWConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Environment variable overrides for secrets
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
		BaseURL:       "http://localhost:8080",
		SessionSecret: "change-me-in-production",
		DBPath:        "/var/lib/wg-route-panel/panel.db",
		WireGuard:     WGConfig{InterfaceName: "wg0", ConfigPath: "/etc/wg-route-panel/wireguard/wg0.conf"},
		Xray:          XrayConfig{BinaryPath: "/usr/local/bin/xray", ConfigPath: "/etc/wg-route-panel/xray/config.json"},
		Geosite:       GeositeConfig{Path: "/etc/wg-route-panel/geosite/geosite.dat", UpdateURL: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"},
		DNS:           DNSConfig{ListenAddr: "127.0.0.1", Port: 5353},
		SOCKS:         SOCKSConfig{ListenAddr: "127.0.0.1", Port: 1080},
		Firewall:      FWConfig{Enabled: true},
		Log:           LogConfig{Level: "info", File: "/var/log/wg-route-panel/panel.log"},
	}
}
