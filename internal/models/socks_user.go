package models

type SocksUser struct {
	ID                     int64
	Username, PasswordHash string
	Enabled                bool
}
