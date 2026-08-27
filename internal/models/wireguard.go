package models

import "time"

type WireGuardStatus struct {
	State, Endpoint, LatestHandshake, LastError string
	RXBytes, TXBytes                            int64
	CheckedAt                                   time.Time
}
