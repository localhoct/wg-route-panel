package repository

import (
	"database/sql"
	_ "embed"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

//go:embed migrations/0001_init.sql
var migration string

func InitDB(path string) (*sql.DB, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0750); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	if _, e = db.Exec(migration); e != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", e)
	}
	return db, nil
}
