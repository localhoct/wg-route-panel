package models

import "time"

type User struct {
	ID                                 int64
	Username, PasswordHash, TOTPSecret string
	TOTPEnabled, Enabled               bool
	CreatedAt, UpdatedAt               time.Time
}
