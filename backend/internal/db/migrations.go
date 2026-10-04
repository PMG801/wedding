package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var migrationName = regexp.MustCompile(`^([0-9]{4})_[a-z0-9_]+\.sql$`)

var migrationFiles = func() fs.FS {
	files, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		panic(fmt.Sprintf("open embedded migrations: %v", err))
	}
	return files
}()

type migration struct {
	version int
	name    string
}

func applyMigrations(ctx context.Context, database *sql.DB, files fs.FS) error {
	migrations, err := listMigrations(files)
	if err != nil {
		return err
	}

	var currentVersion int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read PRAGMA user_version: %w", err)
	}
	if currentVersion < 0 {
		return fmt.Errorf("invalid SQLite schema version %d", currentVersion)
	}
	if len(migrations) > 0 && currentVersion > migrations[len(migrations)-1].version {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, migrations[len(migrations)-1].version)
	}
	if currentVersion > len(migrations) {
		return fmt.Errorf("database schema version %d has no corresponding migration", currentVersion)
	}

	for _, item := range migrations {
		if item.version <= currentVersion {
			continue
		}
		contents, err := fs.ReadFile(files, item.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", item.name, err)
		}
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", item.name, err)
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", item.name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", item.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s version: %w", item.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", item.name, err)
		}
	}
	return nil
}

func listMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		matches := migrationName.FindStringSubmatch(name)
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil || version == 0 {
			return nil, fmt.Errorf("invalid migration version in %q", name)
		}
		wantVersion := len(migrations) + 1
		if version != wantVersion {
			return nil, fmt.Errorf("migration %s has version %d; expected contiguous version %04d", name, version, wantVersion)
		}
		if path.Base(name) != name {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}
		migrations = append(migrations, migration{version: version, name: name})
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no embedded SQL migrations found")
	}
	return migrations, nil
}
