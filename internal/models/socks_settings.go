package models

// SocksSettings is the operator-tunable SOCKS5/mixed inbound configuration,
// stored as key/value pairs in app_settings and edited entirely from the
// panel's SOCKS5 page.
type SocksSettings struct {
	Enabled    bool
	ListenAddr string
	Port       int
	Username   string
	Password   string
}
