package repository

import (
	"context"
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/models"
)

func Audit(ctx context.Context, db *sql.DB, uid int64, a, d, ip string) {
	_, _ = db.ExecContext(ctx, "INSERT INTO audit_logs(user_id,action,details,ip_address) VALUES(NULLIF(?,0),?,?,?)", uid, a, d, ip)
}
func Audits(ctx context.Context, db *sql.DB) ([]models.Audit, error) {
	rows, e := db.QueryContext(ctx, "SELECT id,COALESCE(user_id,0),action,COALESCE(details,''),COALESCE(ip_address,''),created_at FROM audit_logs ORDER BY id DESC LIMIT 200")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []models.Audit
	for rows.Next() {
		var a models.Audit
		if e = rows.Scan(&a.ID, &a.UserID, &a.Action, &a.Details, &a.IPAddress, &a.CreatedAt); e != nil {
			return nil, e
		}
		o = append(o, a)
	}
	return o, rows.Err()
}
