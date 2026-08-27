package models

import "time"

type ACLRule struct {
	ID               int64
	Scope, CIDR      string
	IPVersion        int
	Enabled, Applied bool
	CreatedAt        time.Time
}
