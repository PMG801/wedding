package db

import (
	"context"
	"database/sql"
	"io/fs"
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
	if version != 3 {
		t.Fatalf("user_version = %d; want 3", version)
	}

	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q; want wal", journalMode)
	}

	wantTables := []string{"media", "media_likes", "pinned_items", "settings"}
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

	wantIndexes := []string{"media_hidden_cursor", "media_visible_cursor", "media_visible_popularity"}
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
		if name == "media_visible_popularity" {
			if !strings.Contains(definition, "on media (like_count desc, confirmed_at desc, id desc)") || !strings.Contains(definition, "where visibility = 'visible' and like_count > 0") {
				t.Errorf("index %s has unexpected ranking definition: %s", name, definition)
			}
			continue
		}
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
			{name: "synchronous", want: 2}, // FULL
			{name: "journal_size_limit", want: 67108864},
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

// TestMigration0002AddsLikesToExistingVersionOneDatabase upgrades a database
// that already holds media rows and was created at schema version 1.
func TestMigration0002AddsLikesToExistingVersionOneDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v1.db")
	const mediaID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const guestID = "0123456789abcdef0123456789abcdef"

	v1Contents, err := fs.ReadFile(migrationFiles, "0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, legacy, fstest.MapFS{"0001_initial.sql": &fstest.MapFile{Data: v1Contents}}); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES (?, 'legacy.jpg', 'image/jpeg', 3, 1700000000000)", mediaID); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() upgrade error = %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("user_version after upgrade = %d; want 3", version)
	}
	var likeCount int
	if err := database.QueryRowContext(ctx, "SELECT like_count FROM media WHERE id = ?", mediaID).Scan(&likeCount); err != nil {
		t.Fatal(err)
	}
	if likeCount != 0 {
		t.Fatalf("existing media like_count = %d; want 0", likeCount)
	}

	if _, err := database.ExecContext(ctx, "INSERT INTO media_likes (media_id, guest_id, created_at) VALUES (?, ?, 1700000000001)", mediaID, guestID); err != nil {
		t.Fatalf("insert valid like: %v", err)
	}
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "duplicate guest like", sql: "INSERT INTO media_likes (media_id, guest_id, created_at) VALUES ('" + mediaID + "', '" + guestID + "', 1)"},
		{name: "unknown media", sql: "INSERT INTO media_likes (media_id, guest_id, created_at) VALUES ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'fedcba9876543210fedcba9876543210', 1)"},
		{name: "non-hex guest", sql: "INSERT INTO media_likes (media_id, guest_id, created_at) VALUES ('" + mediaID + "', 'GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG', 1)"},
		{name: "negative like count", sql: "UPDATE media SET like_count = -1 WHERE id = '" + mediaID + "'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, test.sql); err == nil {
				t.Error("statement unexpectedly succeeded")
			}
		})
	}
}

// TestMigration0003AddsPinnedItemsToVersionTwoDatabase upgrades a database at
// schema version 2 that already holds media and likes; those rows must survive,
// and the pinned_items constraints must hold (ADR-0013, data-model 0003).
func TestMigration0003AddsPinnedItemsToVersionTwoDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")
	const mediaID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const guestID = "0123456789abcdef0123456789abcdef"

	legacyFiles := fstest.MapFS{}
	for _, name := range []string{"0001_initial.sql", "0002_media_likes.sql"} {
		contents, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		legacyFiles[name] = &fstest.MapFile{Data: contents}
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, legacy, legacyFiles); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES (?, 'legacy.jpg', 'image/jpeg', 3, 1700000000000)", mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, "INSERT INTO media_likes (media_id, guest_id, created_at) VALUES (?, ?, 1700000000001)", mediaID, guestID); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, "UPDATE media SET like_count = 1 WHERE id = ?", mediaID); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() upgrade error = %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("user_version after upgrade = %d; want 3", version)
	}
	var likeCount, likeRows, mediaRows int
	if err := database.QueryRowContext(ctx, "SELECT like_count FROM media WHERE id = ?", mediaID).Scan(&likeCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM media_likes WHERE media_id = ?", mediaID).Scan(&likeRows); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM media").Scan(&mediaRows); err != nil {
		t.Fatal(err)
	}
	if likeCount != 1 || likeRows != 1 || mediaRows != 1 {
		t.Fatalf("after upgrade media=%d likes=%d like_count=%d; want 1, 1, 1", mediaRows, likeRows, likeCount)
	}

	overLong := strings.Repeat("ñ", 121)
	if _, err := database.ExecContext(ctx, "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p1', ?, '', NULL, 0, 1, 1)", strings.Repeat("ñ", 120)); err != nil {
		t.Fatalf("insert 120-character title: %v", err)
	}
	if _, err := database.ExecContext(ctx, "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p2', ?, '', NULL, 1, 1, 1)", overLong); err == nil {
		t.Error("121-character title unexpectedly accepted")
	}
	for _, test := range []struct {
		name string
		sql  string
		args []any
	}{
		{name: "empty title", sql: "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p3', '', '', NULL, 1, 1, 1)"},
		{name: "over-long body", sql: "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p4', 'ok', ?, NULL, 1, 1, 1)", args: []any{strings.Repeat("x", 2001)}},
		{name: "negative position", sql: "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p5', 'ok', '', NULL, -1, 1, 1)"},
		{name: "duplicate position", sql: "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p6', 'ok', '', NULL, 0, 1, 1)"},
		{name: "unknown media", sql: "INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at) VALUES ('p7', 'ok', '', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 1, 1, 1)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, test.sql, test.args...); err == nil {
				t.Error("insert unexpectedly succeeded")
			}
		})
	}
}
