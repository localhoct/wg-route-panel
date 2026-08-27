package models

import "time"

type Audit struct {
	ID, UserID                 int64
	Action, Details, IPAddress string
	CreatedAt                  time.Time
}
