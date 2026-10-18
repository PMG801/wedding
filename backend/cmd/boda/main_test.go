package main

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/upload"
	"golang.org/x/crypto/bcrypt"
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

func TestAppHandlerWiresAdminSwitchIntoUploadsAndPersistsIt(t *testing.T) {
	root := t.TempDir()
	configureTestEnvironment(t, root)
	hash, err := bcrypt.GenerateFromPassword([]byte("boda-admin"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BODA_ADMIN_PASSWORD_HASH", string(hash))
	ctx := context.Background()
	database, cfg, paths, err := initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	photos, err := upload.NewStore(database, paths, upload.Options{MaxBytes: cfg.MaxPhotoBytes, MaxVideoBytes: cfg.MaxVideoBytes, MaxConcurrent: cfg.MaxConcurrentUploads})
	if err != nil {
		t.Fatal(err)
	}
	gallery, err := media.NewGallery(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAppHandler(ctx, database, cfg, photos, gallery)
	if err != nil {
		t.Fatalf("newAppHandler() = %v", err)
	}

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"boda-admin"}`)))
	if login.Code != http.StatusNoContent {
		t.Fatalf("admin login = %d; want 204", login.Code)
	}
	var adminCookie *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == auth.AdminCookieName {
			adminCookie = cookie
		}
	}
	if adminCookie == nil {
		t.Fatal("login did not set the admin cookie")
	}

	disable := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(`{"uploadsEnabled":false}`))
	disable.AddCookie(adminCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, disable)
	if response.Code != http.StatusOK {
		t.Fatalf("disable uploads = %d %q; want 200", response.Code, response.Body.String())
	}

	guest := auth.NewGuestCookie([]byte(cfg.SessionKey), time.Now())
	photo := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	putRequest := httptest.NewRequest(http.MethodPut, "/api/media/123e4567-e89b-42d3-a456-426614174000", bytes.NewReader(photo))
	putRequest.AddCookie(guest)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, putRequest)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("PUT while paused = %d %q; want 503 uploads_paused", response.Code, response.Body.String())
	}

	// A fresh handler on the same database must start with the persisted switch.
	restarted, err := newAppHandler(ctx, database, cfg, photos, gallery)
	if err != nil {
		t.Fatal(err)
	}
	status := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	status.AddCookie(adminCookie)
	response = httptest.NewRecorder()
	restarted.ServeHTTP(response, status)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"uploadsEnabled":false`) {
		t.Fatalf("status after restart = %d %q; want uploadsEnabled false", response.Code, response.Body.String())
	}
}
