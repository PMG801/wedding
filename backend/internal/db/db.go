// Package db opens the application's SQLite database and applies its schema.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const maxOpenConnections = 8

// Open opens path, configures SQLite, and applies all embedded migrations.
// Connection-local pragmas are encoded in the DSN so every pooled connection
// receives them, not just the connection used during initialization.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	dsnURL := &url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}
	query := dsnURL.Query()
	query.Set("mode", "rwc")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_pragma", "foreign_keys(ON)")
	dsnURL.RawQuery = query.Encode()

	database, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(maxOpenConnections)
	database.SetMaxIdleConns(maxOpenConnections)

	closeOnError := func(err error) (*sql.DB, error) {
		_ = database.Close()
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("connect to SQLite database: %w", err))
	}

	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return closeOnError(fmt.Errorf("enable SQLite WAL mode: %w", err))
	}
	if journalMode != "wal" {
		return closeOnError(fmt.Errorf("enable SQLite WAL mode: got %q", journalMode))
	}

	if err := applyMigrations(ctx, database, migrationFiles); err != nil {
		return closeOnError(fmt.Errorf("apply SQLite migrations: %w", err))
	}
	return database, nil
}
