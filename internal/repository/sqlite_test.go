package repository

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrationsAndDNSRules(t *testing.T) {
	db, e := InitDB(filepath.Join(t.TempDir(), "panel.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	if e = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&n); e != nil || n < 12 {
		t.Fatalf("migration tables=%d err=%v", n, e)
	}
	if e = AddDNSRule(context.Background(), db, "example.com", "block", ""); e != nil {
		t.Fatal(e)
	}
	r, e := DNSRules(context.Background(), db)
	if e != nil || len(r) != 1 || r[0].Action != "block" {
		t.Fatalf("rules=%v err=%v", r, e)
	}
}
