package models

type DNSSettings struct {
	ListenAddr     string
	Port           int
	DirectUpstream string
	ProxyUpstream  string
}
