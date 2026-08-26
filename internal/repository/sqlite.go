package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	migrationFile := "migrations/0001_init.sql"
	// Fallback for when running from different directories
	if _, err := os.Stat(migrationFile); os.IsNotExist(err) {
		migrationFile = "../migrations/0001_init.sql"
	}

	data, err := os.ReadFile(migrationFile)
	if err != nil {
		// If migration file is not found, we skip for now (or embed it in production)
		return nil 
	}

	_, err = db.Exec(string(data))
	if err != nil {
		return fmt.Errorf("migration execution failed: %w", err)
	}

	return nil
}
