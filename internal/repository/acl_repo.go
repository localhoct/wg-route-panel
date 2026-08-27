package repository

import (
	"context"
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/models"
)

func ACLRules(ctx context.Context, db *sql.DB) ([]models.ACLRule, error) {
	rows, e := db.QueryContext(ctx, "SELECT id,scope,cidr,ip_version,enabled,applied,created_at FROM acl_rules ORDER BY scope,cidr")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.ACLRule
	for rows.Next() {
		var x models.ACLRule
		if e = rows.Scan(&x.ID, &x.Scope, &x.CIDR, &x.IPVersion, &x.Enabled, &x.Applied, &x.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func AddACL(ctx context.Context, db *sql.DB, s, c string, v int) error {
	_, e := db.ExecContext(ctx, "INSERT INTO acl_rules(scope,cidr,ip_version) VALUES(?,?,?)", s, c, v)
	return e
}
func ACLByID(ctx context.Context, db *sql.DB, id int64) (models.ACLRule, error) {
	var x models.ACLRule
	e := db.QueryRowContext(ctx, "SELECT id,scope,cidr,ip_version,enabled,applied,created_at FROM acl_rules WHERE id=?", id).Scan(&x.ID, &x.Scope, &x.CIDR, &x.IPVersion, &x.Enabled, &x.Applied, &x.CreatedAt)
	return x, e
}
func DeleteACL(ctx context.Context, db *sql.DB, id int64) error {
	_, e := db.ExecContext(ctx, "DELETE FROM acl_rules WHERE id=?", id)
	return e
}
