package repository

import (
	"context"
	"database/sql"
	"time"
)

func CreateSession(ctx context.Context, db *sql.DB, hash string, uid int64, exp time.Time, ip, ua string) error {
	_, e := db.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at,ip_address,user_agent) VALUES(?,?,?,?,?)", hash, uid, exp, ip, ua)
	return e
}
func SessionUser(ctx context.Context, db *sql.DB, h string) (int64, error) {
	var id int64
	e := db.QueryRowContext(ctx, "SELECT user_id FROM sessions WHERE token_hash=? AND expires_at>CURRENT_TIMESTAMP", h).Scan(&id)
	if e == nil {
		_, _ = db.ExecContext(ctx, "UPDATE sessions SET last_seen_at=CURRENT_TIMESTAMP WHERE token_hash=?", h)
	}
	return id, e
}
func DeleteSession(ctx context.Context, db *sql.DB, h string) error {
	_, e := db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", h)
	return e
}
