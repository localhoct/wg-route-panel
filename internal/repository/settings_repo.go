package repository

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/localhoct/wg-route-panel/internal/models"
)

// app_settings is a simple key/value store (see migrations/0001_init.sql)
// holding every operator-tunable runtime setting: DNS listener/upstreams
// and SOCKS5 listener/credentials. Everything that used to require editing
// YAML or SSHing into the box is a row here, edited from the panel's
// Settings/DNS/SOCKS5 pages, and read back by SingBoxService whenever the
// sing-box configuration is regenerated.
const (
	keyDNSListenAddr     = "dns.listen_addr"
	keyDNSPort           = "dns.port"
	keyDNSDirectUpstream = "dns.direct_upstream"
	keyDNSProxyUpstream  = "dns.proxy_upstream"
	keySocksEnabled      = "socks.enabled"
	keySocksListenAddr   = "socks.listen_addr"
	keySocksPort         = "socks.port"
	keySocksUsername     = "socks.username"
	keySocksPassword     = "socks.password"
)

// Defaults: the DNS resolver listens on 0.0.0.0 so it is reachable at the
// server's public IP out of the box (per the requirement that the DNS
// service be usable from the public internet without extra setup); SOCKS5
// stays disabled and loopback-only until an operator turns it on.
const (
	defaultDNSListenAddr     = "0.0.0.0"
	defaultDNSPort           = 53
	defaultDNSDirectUpstream = "1.1.1.1"
	defaultDNSProxyUpstream  = "1.1.1.1"
	defaultSocksListenAddr   = "127.0.0.1"
	defaultSocksPort         = 1080
)

func getSetting(ctx context.Context, db *sql.DB, key, fallback string) (string, error) {
	var v string
	e := db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key=?", key).Scan(&v)
	if e == sql.ErrNoRows {
		return fallback, nil
	}
	if e != nil {
		return "", e
	}
	return v, nil
}
func getSettingInt(ctx context.Context, db *sql.DB, key string, fallback int) (int, error) {
	v, e := getSetting(ctx, db, key, "")
	if e != nil {
		return 0, e
	}
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}
func setSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, e := db.ExecContext(ctx, "INSERT INTO app_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP", key, value)
	return e
}

// DNSSettings returns the current DNS listener/upstream configuration,
// falling back to public-by-default values when unset.
func DNSSettings(ctx context.Context, db *sql.DB) (models.DNSSettings, error) {
	var s models.DNSSettings
	var e error
	if s.ListenAddr, e = getSetting(ctx, db, keyDNSListenAddr, defaultDNSListenAddr); e != nil {
		return s, e
	}
	if s.Port, e = getSettingInt(ctx, db, keyDNSPort, defaultDNSPort); e != nil {
		return s, e
	}
	if s.DirectUpstream, e = getSetting(ctx, db, keyDNSDirectUpstream, defaultDNSDirectUpstream); e != nil {
		return s, e
	}
	if s.ProxyUpstream, e = getSetting(ctx, db, keyDNSProxyUpstream, defaultDNSProxyUpstream); e != nil {
		return s, e
	}
	return s, nil
}

func UpdateDNSSettings(ctx context.Context, db *sql.DB, s models.DNSSettings) error {
	for _, kv := range []struct{ k, v string }{
		{keyDNSListenAddr, s.ListenAddr},
		{keyDNSPort, strconv.Itoa(s.Port)},
		{keyDNSDirectUpstream, s.DirectUpstream},
		{keyDNSProxyUpstream, s.ProxyUpstream},
	} {
		if e := setSetting(ctx, db, kv.k, kv.v); e != nil {
			return e
		}
	}
	return nil
}

// SocksSettings returns the current SOCKS5/mixed inbound configuration.
func SocksSettings(ctx context.Context, db *sql.DB) (models.SocksSettings, error) {
	var s models.SocksSettings
	var e error
	var enabled string
	if enabled, e = getSetting(ctx, db, keySocksEnabled, "0"); e != nil {
		return s, e
	}
	s.Enabled = enabled == "1"
	if s.ListenAddr, e = getSetting(ctx, db, keySocksListenAddr, defaultSocksListenAddr); e != nil {
		return s, e
	}
	if s.Port, e = getSettingInt(ctx, db, keySocksPort, defaultSocksPort); e != nil {
		return s, e
	}
	if s.Username, e = getSetting(ctx, db, keySocksUsername, ""); e != nil {
		return s, e
	}
	if s.Password, e = getSetting(ctx, db, keySocksPassword, ""); e != nil {
		return s, e
	}
	return s, nil
}

func UpdateSocksSettings(ctx context.Context, db *sql.DB, s models.SocksSettings) error {
	enabled := "0"
	if s.Enabled {
		enabled = "1"
	}
	for _, kv := range []struct{ k, v string }{
		{keySocksEnabled, enabled},
		{keySocksListenAddr, s.ListenAddr},
		{keySocksPort, strconv.Itoa(s.Port)},
		{keySocksUsername, s.Username},
		{keySocksPassword, s.Password},
	} {
		if e := setSetting(ctx, db, kv.k, kv.v); e != nil {
			return e
		}
	}
	return nil
}
