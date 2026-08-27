package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr        string        `yaml:"listen_addr"`
	BaseURL           string        `yaml:"base_url"`
	SessionSecret     string        `yaml:"session_secret"`
	DBPath            string        `yaml:"db_path"`
	SecureCookies     bool          `yaml:"secure_cookies"`
	AllowInsecureHTTP bool          `yaml:"allow_insecure_http"`
	TrustedProxies    []string      `yaml:"trusted_proxies"`
	WireGuard         WGConfig      `yaml:"wireguard"`
	SingBox           SingBoxConfig `yaml:"sing_box"`
	Xray              XrayConfig    `yaml:"xray"`
	Geosite           GeositeConfig `yaml:"geosite"`
	DNS               DNSConfig     `yaml:"dns"`
	SOCKS             SOCKSConfig   `yaml:"socks"`
	Firewall          FWConfig      `yaml:"firewall"`
	Log               LogConfig     `yaml:"log"`
}
type WGConfig struct {
	InterfaceName string `yaml:"interface_name"`
	ConfigPath    string `yaml:"config_path"`
	ProbeAddress  string `yaml:"probe_address"`
}
type SingBoxConfig struct {
	BinaryPath    string `yaml:"binary_path"`
	ConfigPath    string `yaml:"config_path"`
	Manager       string `yaml:"manager"`
	SupervisorCtl string `yaml:"supervisorctl_path"`
}
type XrayConfig struct {
	BinaryPath    string `yaml:"binary_path"`
	ConfigPath    string `yaml:"config_path"`
	Manager       string `yaml:"manager"`
	ServiceName   string `yaml:"service_name"`
	SupervisorCtl string `yaml:"supervisorctl_path"`
}
type GeositeConfig struct {
	Path      string `yaml:"path"`
	UpdateURL string `yaml:"update_url"`
	SHA256    string `yaml:"sha256"`
}
type DNSConfig struct {
	ListenAddr     string `yaml:"listen_addr"`
	Port           int    `yaml:"port"`
	DirectUpstream string `yaml:"direct_upstream"`
	TunnelUpstream string `yaml:"tunnel_upstream"`
}
type SOCKSConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
}
type FWConfig struct {
	Enabled bool `yaml:"enabled"`
}
type LogConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Default() *Config {
	return &Config{
		ListenAddr: "127.0.0.1:9090",
		BaseURL:    "http://127.0.0.1:9090",
		DBPath:     "./data/panel.db",
		WireGuard:  WGConfig{InterfaceName: "wg0", ConfigPath: "./data/wg0.conf"},
		SingBox: SingBoxConfig{
			BinaryPath:    "/usr/local/bin/sing-box",
			ConfigPath:    "./data/sing-box.json",
			Manager:       "supervisor",
			SupervisorCtl: "/usr/bin/supervisorctl",
		},
		Xray: XrayConfig{
			BinaryPath:    "/usr/local/bin/xray",
			ConfigPath:    "./data/xray.json",
			Manager:       "supervisor",
			ServiceName:   "xray",
			SupervisorCtl: "/usr/bin/supervisorctl",
		},
		Geosite: GeositeConfig{Path: "./data/geosite.dat", UpdateURL: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"},
		DNS:     DNSConfig{ListenAddr: "127.0.0.1", Port: 5353, DirectUpstream: "1.1.1.1", TunnelUpstream: "8.8.8.8"},
		SOCKS:   SOCKSConfig{ListenAddr: "127.0.0.1", Port: 1080},
		Firewall: FWConfig{
			Enabled: false,
		},
		Log: LogConfig{Level: "info", File: "./data/panel.log"},
	}
}
func Load(path string) (*Config, error) {
	c := Default()
	if b, e := os.ReadFile(path); e == nil {
		if e = yaml.Unmarshal(b, c); e != nil {
			return nil, e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	if v := os.Getenv("PANEL_SESSION_SECRET"); v != "" {
		c.SessionSecret = v
	}
	if v := os.Getenv("PANEL_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("PANEL_SOCKS_USERNAME"); v != "" {
		c.SOCKS.Username = v
	}
	if v := os.Getenv("PANEL_SOCKS_PASSWORD"); v != "" {
		c.SOCKS.Password = v
	}
	if c.SessionSecret == "" {
		if strings.HasPrefix(c.ListenAddr, "127.0.0.1:") {
			b := make([]byte, 32)
			_, _ = rand.Read(b)
			c.SessionSecret = base64.RawURLEncoding.EncodeToString(b)
		} else {
			return nil, errors.New("session_secret or PANEL_SESSION_SECRET is required for non-loopback listening")
		}
	}
	return c, c.Validate()
}
func (c *Config) Validate() error {
	if len(c.SessionSecret) < 32 {
		return errors.New("session secret must be at least 32 characters")
	}
	host, _, e := net.SplitHostPort(c.ListenAddr)
	if e != nil {
		return fmt.Errorf("listen_addr: %w", e)
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("invalid base_url")
	}
	if u.Scheme == "http" && c.SecureCookies {
		return errors.New("secure_cookies must be false when base_url uses HTTP")
	}
	if host != "127.0.0.1" && host != "::1" && u.Scheme != "https" && !c.AllowInsecureHTTP {
		return errors.New("HTTPS base_url required when panel is not loopback-only unless allow_insecure_http is true")
	}
	if c.WireGuard.InterfaceName == "" || strings.ContainsAny(c.WireGuard.InterfaceName, "/ \\;'") {
		return errors.New("invalid WireGuard interface name")
	}
	if c.SOCKS.ListenAddr != "127.0.0.1" && c.SOCKS.ListenAddr != "::1" && (c.SOCKS.Username == "" || c.SOCKS.Password == "") {
		return errors.New("non-loopback SOCKS requires PANEL_SOCKS_USERNAME and PANEL_SOCKS_PASSWORD")
	}
	if c.SingBox.Manager != "supervisor" && c.SingBox.Manager != "systemd" {
		return errors.New("sing_box.manager must be supervisor or systemd")
	}
	if c.Xray.Manager != "supervisor" && c.Xray.Manager != "systemd" {
		return errors.New("xray.manager must be supervisor or systemd")
	}
	if c.SingBox.Manager == "supervisor" && c.SingBox.SupervisorCtl == "" {
		return errors.New("sing_box.supervisorctl_path is required for supervisor")
	}
	if c.Xray.Manager == "supervisor" && c.Xray.SupervisorCtl == "" {
		return errors.New("xray.supervisorctl_path is required for supervisor")
	}
	for _, p := range []string{c.DBPath, c.WireGuard.ConfigPath, c.SingBox.BinaryPath, c.SingBox.ConfigPath, c.Xray.BinaryPath, c.Xray.ConfigPath, c.Geosite.Path} {
		if p == "" {
			return errors.New("required path is empty")
		}
		if filepath.Clean(p) == "." {
			return errors.New("invalid path")
		}
	}
	return nil
}
