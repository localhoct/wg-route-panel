package models

import "time"

// DNSRule is a per-domain routing rule enforced by sing-box's route+dns
// modules. There is no "Applied" flag: the entire sing-box configuration is
// regenerated from every enabled row whenever a rule changes, so an enabled
// rule is always reflected the next time the config is (re)generated.
type DNSRule struct {
	ID                       int64
	Domain, Action, StaticIP string
	Enabled                  bool
	CreatedAt                time.Time
}
