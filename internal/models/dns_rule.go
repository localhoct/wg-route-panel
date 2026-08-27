package models

import "time"

type DNSRule struct {
	ID                       int64
	Domain, Action, StaticIP string
	Enabled, Applied         bool
	CreatedAt                time.Time
}
