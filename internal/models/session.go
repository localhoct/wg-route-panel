package models

import "time"

type Session struct {
	ID, UserID                       int64
	TokenHash, IPAddress, UserAgent  string
	ExpiresAt, CreatedAt, LastSeenAt time.Time
}
