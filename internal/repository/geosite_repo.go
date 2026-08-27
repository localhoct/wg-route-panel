package repository

import (
	"context"
	"database/sql"
	"github.com/localhoct/wg-route-panel/internal/models"
)

func GeositeCategories(ctx context.Context, db *sql.DB, q string) ([]models.GeositeCategory, error) {
	rows, e := db.QueryContext(ctx, "SELECT id,tag,action,selected FROM geosite_categories WHERE tag LIKE ? ORDER BY selected DESC,tag LIMIT 500", "%"+q+"%")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []models.GeositeCategory
	for rows.Next() {
		var x models.GeositeCategory
		if e = rows.Scan(&x.ID, &x.Tag, &x.Action, &x.Selected); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}
func AssignGeosite(ctx context.Context, db *sql.DB, tag, action string, selected bool) error {
	_, e := db.ExecContext(ctx, "INSERT INTO geosite_categories(tag,action,selected) VALUES(?,?,?) ON CONFLICT(tag) DO UPDATE SET action=excluded.action,selected=excluded.selected,updated_at=CURRENT_TIMESTAMP", tag, action, selected)
	return e
}
