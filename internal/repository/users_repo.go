package repository

import (
	"context"
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/models"
)

func CreateUser(ctx context.Context, db *sql.DB, u, h string) error {
	_, e := db.ExecContext(ctx, "INSERT INTO users(username,password_hash) VALUES(?,?)", u, h)
	return e
}
func UserByName(ctx context.Context, db *sql.DB, n string) (models.User, error) {
	var u models.User
	var te, en int
	e := db.QueryRowContext(ctx, "SELECT id,username,password_hash,COALESCE(totp_secret,''),totp_enabled,enabled FROM users WHERE username=?", n).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &te, &en)
	u.TOTPEnabled = te != 0
	u.Enabled = en != 0
	return u, e
}
func UpdatePassword(ctx context.Context, db *sql.DB, id int64, h string) error {
	_, e := db.ExecContext(ctx, "UPDATE users SET password_hash=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", h, id)
	return e
}
