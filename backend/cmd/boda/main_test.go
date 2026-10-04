package main

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configureTestEnvironment(t *testing.T, root string) {
	t.Helper()
	t.Setenv("BODA_DATA_DIR", root)
	t.Setenv("BODA_EVENT_TOKEN", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("BODA_ADMIN_PASSWORD_HASH", "$2b$12$"+strings.Repeat("a", 53))
	t.Setenv("BODA_SESSION_KEY", "0123456789abcdef0123456789abcdef")
}

func TestRunCheckInitializesStorageAndSQLite(t *testing.T) {
	root := t.TempDir()
	configureTestEnvironment(t, root)

	if err := runCheck(); err != nil {
		t.Fatalf("runCheck() = %v; want nil", err)
	}
	for _, name := range []string{"data", "tmp", "originals", "derived", "hidden", "quarantine"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
			t.Errorf("managed directory %q: info=%v, err=%v", name, info, err)
		}
	}

	database, err := sql.Open("sqlite", filepath.Join(root, "data", "boda.db"))
	if err != nil {
		t.Fatalf("open initialized database: %v", err)
	}
	defer database.Close()
	var version int
	if err := database.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d; want 1", version)
	}
}

func TestRunCheckRejectsInvalidConfigurationWithoutLeakingSecret(t *testing.T) {
	root := t.TempDir()
	configureTestEnvironment(t, root)
	secret := "not-a-valid-secret-value"
	t.Setenv("BODA_EVENT_TOKEN", secret)

	err := runCheck()
	if err == nil {
		t.Fatal("runCheck() error = nil; want invalid configuration error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("runCheck() leaked configured secret: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(statErr) {
		t.Errorf("invalid configuration created managed storage: stat error = %v", statErr)
	}
}

func TestRunCheckRejectsMissingDataRootWithoutCreatingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-root")
	configureTestEnvironment(t, root)

	if err := runCheck(); err == nil {
		t.Fatal("runCheck() error = nil; want missing data root error")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("missing data root was created or unexpected stat error: %v", err)
	}
}

func TestRunCheckRejectsDatabasePathThatIsDirectory(t *testing.T) {
	root := t.TempDir()
	configureTestEnvironment(t, root)
	if err := os.MkdirAll(filepath.Join(root, "data", "boda.db"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := runCheck(); err == nil {
		t.Fatal("runCheck() error = nil; want database initialization error")
	}
}

func TestRunServeUsesInitializationAndReturnsListenerFailure(t *testing.T) {
	root := t.TempDir()
	configureTestEnvironment(t, root)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test listener: %v", err)
	}
	defer listener.Close()
	t.Setenv("BODA_ADDR", listener.Addr().String())

	if err := runServe(); err == nil {
		t.Fatal("runServe() error = nil; want listener failure")
	}
	if _, err := os.Stat(filepath.Join(root, "data", "boda.db")); err != nil {
		t.Errorf("runServe() did not initialize the database: %v", err)
	}
}
