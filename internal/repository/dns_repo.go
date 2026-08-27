package repository

import (
	"context"
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/models"
)

func DNSRules(ctx context.Context, db *sql.DB) ([]models.DNSRule, error) {
	rows, e := db.QueryContext(ctx, "SELECT id,domain,action,COALESCE(static_ip,''),enabled,applied,created_at FROM dns_rules ORDER BY domain")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.DNSRule
	for rows.Next() {
		var x models.DNSRule
		if e = rows.Scan(&x.ID, &x.Domain, &x.Action, &x.StaticIP, &x.Enabled, &x.Applied, &x.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func AddDNSRule(ctx context.Context, db *sql.DB, d, a, ip string) error {
	_, e := db.ExecContext(ctx, "INSERT INTO dns_rules(domain,action,static_ip) VALUES(?,?,?)", d, a, ip)
	return e
}
func DeleteDNSRule(ctx context.Context, db *sql.DB, id int64) error {
	_, e := db.ExecContext(ctx, "DELETE FROM dns_rules WHERE id=?", id)
	return e
}
