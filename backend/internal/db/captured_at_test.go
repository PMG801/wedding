package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// migrationsUpTo returns the embedded migrations whose version is at most n,
// so a database can be brought to an older schema and then upgraded.
func migrationsUpTo(t *testing.T, n int) fs.FS {
	t.Helper()
	entries, err := fs.ReadDir(migrationFiles, ".")
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for _, entry := range entries {
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%04d_", &version); err != nil {
			t.Fatal(err)
		}
		if version <= n {
			contents, err := fs.ReadFile(migrationFiles, entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = &fstest.MapFile{Data: contents}
		}
	}
	return files
}

func openRawDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "raw.db"))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestMigration0004KeepsExistingRowsWithoutCapturedAt(t *testing.T) {
	ctx := context.Background()
	database := openRawDatabase(t)
	if err := applyMigrations(ctx, database, migrationsUpTo(t, 3)); err != nil {
		t.Fatalf("apply 0001-0003: %v", err)
	}
	const id = "123e4567-e89b-42d3-a456-426614174000"
	if _, err := database.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES (?, 'a.jpg', 'image/jpeg', 4, 1790000000000)", id); err != nil {
		t.Fatal(err)
	}

	if err := applyMigrations(ctx, database, migrationFiles); err != nil {
		t.Fatalf("apply 0004 over existing data: %v", err)
	}
	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 4 {
		t.Fatalf("user_version = %d; want 4", version)
	}
	var captured sql.NullString
	if err := database.QueryRowContext(ctx, "SELECT captured_at FROM media WHERE id = ?", id).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if captured.Valid {
		t.Fatalf("captured_at of existing row = %q; want NULL", captured.String)
	}
}

func TestMigration0004CheckAcceptsOnlyTheCapturedAtShape(t *testing.T) {
	ctx := context.Background()
	database := openRawDatabase(t)
	if err := applyMigrations(ctx, database, migrationFiles); err != nil {
		t.Fatal(err)
	}
	next := 0
	insert := func(captured any) error {
		next++
		_, err := database.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, captured_at) VALUES (?, ?, 'image/jpeg', 4, 1, ?)", fmt.Sprintf("row-%d", next), fmt.Sprintf("s-%d.jpg", next), captured)
		return err
	}
	for _, value := range []any{"2026-10-18T18:00:00", "1999-12-31T23:59:59", nil} {
		if err := insert(value); err != nil {
			t.Fatalf("insert captured_at %v rejected: %v; want accepted", value, err)
		}
	}
	for _, value := range []string{"2026-10-18 18:00:00", "2026-10-18T18:00", "2026-10-18T18:00:00Z", "2026-10-18T18:00:00.000", "x", ""} {
		if err := insert(value); err == nil {
			t.Fatalf("insert captured_at %q accepted; want CHECK rejection", value)
		}
	}
}

func TestMigration0004AddsPartialCapturedIndex(t *testing.T) {
	ctx := context.Background()
	database := openRawDatabase(t)
	if err := applyMigrations(ctx, database, migrationFiles); err != nil {
		t.Fatal(err)
	}
	var definition string
	if err := database.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE type = 'index' AND name = 'media_visible_captured'").Scan(&definition); err != nil {
		t.Fatalf("media_visible_captured index missing: %v", err)
	}
	for _, want := range []string{"(captured_at, id)", "visibility = 'visible'", "captured_at IS NOT NULL"} {
		if !strings.Contains(definition, want) {
			t.Fatalf("index definition %q lacks %q", definition, want)
		}
	}
}
