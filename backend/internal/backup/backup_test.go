package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/db"
)

func openDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	root := t.TempDir()
	database, err := db.Open(context.Background(), filepath.Join(root, "boda.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, root
}

func TestRunWritesAConsistentCopyReadableAsSQLite(t *testing.T) {
	database, root := openDB(t)
	if _, err := database.Exec("INSERT INTO settings (key, value, updated_at) VALUES ('k', 'v', 1)"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "backup")
	now := time.Date(2026, 9, 26, 18, 30, 12, 0, time.UTC)
	path, err := Run(context.Background(), database, dir, 3, now)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if want := filepath.Join(dir, "boda-20260926T183012Z.db"); path != want {
		t.Fatalf("Run() path = %q; want %q", path, want)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("backup dir = %v, %v; want mode 0700", info, err)
	}
	copyDB, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var value string
	if err := copyDB.QueryRow("SELECT value FROM settings WHERE key = 'k'").Scan(&value); err != nil || value != "v" {
		t.Fatalf("backup row = %q, %v; want v", value, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestRunKeepsOnlyTheNewestCopies(t *testing.T) {
	database, root := openDB(t)
	dir := filepath.Join(root, "backup")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 20, 3, 0, 0, 0, time.UTC)
	for day := 0; day < 5; day++ {
		if _, err := Run(context.Background(), database, dir, 3, start.AddDate(0, 0, day)); err != nil {
			t.Fatalf("Run(day %d) = %v", day, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	want := []string{"boda-20260922T030000Z.db", "boda-20260923T030000Z.db", "boda-20260924T030000Z.db", "notes.txt"}
	if len(names) != len(want) {
		t.Fatalf("backup dir = %v; want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("backup dir = %v; want %v", names, want)
		}
	}
}

func TestRunRejectsANonPositiveKeep(t *testing.T) {
	database, root := openDB(t)
	if _, err := Run(context.Background(), database, filepath.Join(root, "backup"), 0, time.Now()); err == nil {
		t.Fatal("Run() accepted keep = 0")
	}
}
