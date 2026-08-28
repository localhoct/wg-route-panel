package models

// DNSSettings is the operator-tunable DNS listener/upstream configuration,
// stored as key/value pairs in app_settings and edited entirely from the
// panel's DNS page. ListenAddr defaults to 0.0.0.0 so the DNS resolver is
// reachable at the server's public IP without any shell access.
type DNSSettings struct {
	ListenAddr     string
	Port           int
	DirectUpstream string
	ProxyUpstream  string
}
