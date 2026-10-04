package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestOpenConfiguresPoolAndAppliesSchemaOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "boda.db")

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("user_version = %d; want 1", version)
	}

	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q; want wal", journalMode)
	}

	wantTables := []string{"media", "settings"}
	rows, err := database.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("tables = %v; want exactly %v", tables, wantTables)
	}

	wantIndexes := []string{"media_hidden_cursor", "media_visible_cursor"}
	indexRows, err := database.QueryContext(ctx, "SELECT name, sql FROM sqlite_schema WHERE type = 'index' AND tbl_name = 'media' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var indexes []string
	for indexRows.Next() {
		var name, definition string
		if err := indexRows.Scan(&name, &definition); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, name)
		definition = strings.ToLower(definition)
		if !strings.Contains(definition, "on media (confirmed_at desc, id desc)") {
			t.Errorf("index %s has unexpected cursor ordering: %s", name, definition)
		}
		state := map[string]string{"media_hidden_cursor": "hidden", "media_visible_cursor": "visible"}[name]
		if !strings.Contains(definition, "where visibility = '"+state+"'") {
			t.Errorf("index %s has unexpected partial predicate: %s", name, definition)
		}
	}
	if err := indexRows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := indexRows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(indexes, wantIndexes) {
		t.Fatalf("media indexes = %v; want %v", indexes, wantIndexes)
	}

	// Hold several pooled connections at once so each connection's local
	// pragmas are checked, rather than repeatedly reusing one connection.
	connections := make([]*sql.Conn, 0, 4)
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	for range 4 {
		conn, err := database.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire pooled connection: %v", err)
		}
		connections = append(connections, conn)
		for _, pragma := range []struct {
			name string
			want int
		}{
			{name: "busy_timeout", want: 5000},
			{name: "synchronous", want: 1}, // NORMAL
			{name: "foreign_keys", want: 1},
		} {
			var got int
			if err := conn.QueryRowContext(ctx, "PRAGMA "+pragma.name).Scan(&got); err != nil {
				t.Fatalf("read %s: %v", pragma.name, err)
			}
			if got != pragma.want {
				t.Errorf("connection %d %s = %d; want %d", len(connections), pragma.name, got, pragma.want)
			}
		}
	}

	if _, err := database.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES ('m1', 'one.jpg', 'image/jpeg', 12, 1700000000000)"); err != nil {
		t.Fatalf("insert valid media: %v", err)
	}
	var visibility string
	if err := database.QueryRowContext(ctx, "SELECT visibility FROM media WHERE id = 'm1'").Scan(&visibility); err != nil {
		t.Fatal(err)
	}
	if visibility != "visible" {
		t.Errorf("default visibility = %q; want visible", visibility)
	}
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "nonpositive size", sql: "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES ('m2', 'two.jpg', 'image/jpeg', 0, 1700000000000)"},
		{name: "invalid visibility", sql: "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES ('m3', 'three.jpg', 'image/jpeg', 1, 1700000000000, 'uploading')"},
		{name: "duplicate storage name", sql: "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES ('m4', 'one.jpg', 'image/jpeg', 1, 1700000000000)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, test.sql); err == nil {
				t.Error("insert unexpectedly succeeded")
			}
		})
	}

	// A second open must keep the schema version and data, with no duplicate migration effects.
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.QueryRowContext(ctx, "SELECT count(*) FROM media").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("media count after repeated open = %d; want 1", count)
	}
}

func TestMigrationsRollbackFailureAndRejectNewerVersion(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	failingMigrations := fstest.MapFS{
		"0001_partial.sql": &fstest.MapFile{Data: []byte("CREATE TABLE should_rollback (id INTEGER); THIS IS NOT SQL;")},
	}
	if err := applyMigrations(ctx, database, failingMigrations); err == nil {
		t.Fatal("applyMigrations() error = nil; want migration failure")
	}
	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Errorf("user_version after failed migration = %d; want 0", version)
	}
	var tableCount int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='should_rollback'").Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Errorf("partial migration table count = %d; want 0", tableCount)
	}

	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	availableMigrations := fstest.MapFS{
		"0001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	if err := applyMigrations(ctx, database, availableMigrations); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("applyMigrations() error = %v; want newer-version rejection", err)
	}
}
