package models

type DNSServer struct {
	ID                 int64
	Name, Address      string
	ViaTunnel, Enabled bool
}
