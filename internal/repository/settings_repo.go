package repository

import (
	"context"
	"database/sql"

	"github.com/localhoct/wg-route-panel/internal/models"
)

// DNSSettings returns the singleton DNS listener/upstream configuration.
// These values are entirely managed from the web panel (Settings/DNS
// pages) so operators never need shell access to change them.
func DNSSettings(ctx context.Context, db *sql.DB) (models.DNSSettings, error) {
	var s models.DNSSettings
	e := db.QueryRowContext(ctx, "SELECT listen_addr,port,direct_upstream,proxy_upstream FROM dns_settings WHERE id=1").
		Scan(&s.ListenAddr, &s.Port, &s.DirectUpstream, &s.ProxyUpstream)
	return s, e
}

func UpdateDNSSettings(ctx context.Context, db *sql.DB, s models.DNSSettings) error {
	_, e := db.ExecContext(ctx, "UPDATE dns_settings SET listen_addr=?,port=?,direct_upstream=?,proxy_upstream=?,updated_at=CURRENT_TIMESTAMP WHERE id=1",
		s.ListenAddr, s.Port, s.DirectUpstream, s.ProxyUpstream)
	return e
}

// SocksSettings returns the singleton SOCKS/mixed inbound configuration.
func SocksSettings(ctx context.Context, db *sql.DB) (models.SocksSettings, error) {
	var s models.SocksSettings
	e := db.QueryRowContext(ctx, "SELECT listen_addr,port,username,password FROM socks_settings WHERE id=1").
		Scan(&s.ListenAddr, &s.Port, &s.Username, &s.Password)
	return s, e
}

func UpdateSocksSettings(ctx context.Context, db *sql.DB, s models.SocksSettings) error {
	_, e := db.ExecContext(ctx, "UPDATE socks_settings SET listen_addr=?,port=?,username=?,password=?,updated_at=CURRENT_TIMESTAMP WHERE id=1",
		s.ListenAddr, s.Port, s.Username, s.Password)
	return e
}
