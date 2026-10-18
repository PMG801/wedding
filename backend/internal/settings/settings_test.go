package settings

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/PMG801/wedding/internal/db"
)

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "boda.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestUploadsEnabledDefaultsToTrueWhenRowIsMissing(t *testing.T) {
	database := openTestDatabase(t)
	store, err := Open(context.Background(), database)
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	if !store.UploadsEnabled() {
		t.Fatal("UploadsEnabled() = false with no settings row; want true")
	}
	var rows int
	if err := database.QueryRow("SELECT COUNT(*) FROM settings").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("opening the store wrote %d settings rows (err %v); want none", rows, err)
	}
}

func TestUploadsToggleIsPersistedAndSeenByNewStoreInstances(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	first, err := Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}

	if err := first.SetUploadsEnabled(ctx, false); err != nil {
		t.Fatalf("SetUploadsEnabled(false) = %v", err)
	}
	if first.UploadsEnabled() {
		t.Fatal("cached value stayed enabled after disabling")
	}
	var value string
	var updatedAt int64
	if err := database.QueryRow("SELECT value, updated_at FROM settings WHERE key = 'uploads_enabled'").Scan(&value, &updatedAt); err != nil {
		t.Fatalf("read persisted row: %v", err)
	}
	if value != "false" || updatedAt <= 0 {
		t.Fatalf("persisted row = %q at %d; want \"false\" with a timestamp", value, updatedAt)
	}

	reopened, err := Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.UploadsEnabled() {
		t.Fatal("new store instance loaded enabled; want the persisted false")
	}
	if err := reopened.SetUploadsEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	third, err := Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if !third.UploadsEnabled() {
		t.Fatal("store instance after re-enabling loaded disabled")
	}
}

func TestSetRejectsKeysOutsideAllowListAndInvalidValues(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	store, err := Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		key   string
		value string
		want  error
	}{
		{"unknown key", "admin_password_hash", "x", ErrUnknownKey},
		{"empty key", "", "true", ErrUnknownKey},
		{"non-boolean value", UploadsEnabledKey, "maybe", ErrInvalidValue},
		{"uppercase boolean", UploadsEnabledKey, "TRUE", ErrInvalidValue},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Set(ctx, test.key, test.value); !errors.Is(err, test.want) {
				t.Fatalf("Set(%q, %q) = %v; want %v", test.key, test.value, err, test.want)
			}
			if _, _, err := store.Get(ctx, test.key); test.key != UploadsEnabledKey && !errors.Is(err, ErrUnknownKey) {
				t.Fatalf("Get(%q) = %v; want ErrUnknownKey", test.key, err)
			}
		})
	}
	var rows int
	if err := database.QueryRow("SELECT COUNT(*) FROM settings").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rejected writes stored %d rows (err %v); want none", rows, err)
	}
	if !store.UploadsEnabled() {
		t.Fatal("rejected writes changed the cached switch")
	}
}

func TestOpenRejectsStoredValueThatIsNotBoolean(t *testing.T) {
	database := openTestDatabase(t)
	if _, err := database.Exec("INSERT INTO settings (key, value, updated_at) VALUES ('uploads_enabled', 'sometimes', 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), database); err == nil {
		t.Fatal("Open() accepted a non-boolean uploads_enabled value; want an error")
	}
}

func TestFailedWriteKeepsCachedSwitchUnchanged(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	store, err := Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetUploadsEnabled(ctx, false); err == nil {
		t.Fatal("SetUploadsEnabled() on a closed database = nil; want error")
	}
	if !store.UploadsEnabled() {
		t.Fatal("cache changed although the write did not persist")
	}
}

func TestSwitchReadsAreSafeDuringConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, openTestDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(enabled bool) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = store.SetUploadsEnabled(ctx, enabled)
			}
		}(i%2 == 0)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = store.UploadsEnabled()
			}
		}()
	}
	wg.Wait()
}
