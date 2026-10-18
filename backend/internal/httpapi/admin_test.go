package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/settings"
	"github.com/PMG801/wedding/internal/upload"
	"golang.org/x/crypto/bcrypt"
)

const testAdminPassword = "correct horse"

type fakeDisk struct {
	usage upload.DiskUsage
	err   error
}

func (d *fakeDisk) DiskUsage() (upload.DiskUsage, error) { return d.usage, d.err }

// recordingClock makes login delays observable without real sleeps.
type recordingClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *recordingClock) Now() time.Time { return c.now }

func (c *recordingClock) Sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

type adminFixture struct {
	handler  http.Handler
	settings *settings.Store
	disk     *fakeDisk
	clock    *recordingClock
}

func openAdminTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "boda.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func newAdminFixture(t *testing.T, photos photoUploader, gallery mediaLister) *adminFixture {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.Open(context.Background(), openAdminTestDatabase(t))
	if err != nil {
		t.Fatalf("open settings: %v", err)
	}
	disk := &fakeDisk{usage: upload.DiskUsage{FreeBytes: 10 << 30, ReservedBytes: 1 << 30, MinFreeBytes: 5 << 30}}
	clock := &recordingClock{now: time.Now()}
	throttle := auth.NewLoginThrottle(auth.ThrottleOptions{Now: clock.Now, Sleep: clock.Sleep})
	handler := NewHandler(testEventToken, []byte(testSessionKey), photos, 2, time.Second, 1024, gallery,
		WithAdmin(string(hash), store, disk), WithLoginThrottle(throttle))
	return &adminFixture{handler: handler, settings: store, disk: disk, clock: clock}
}

func adminRequest(method, path, body string, cookies ...*http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	return request
}

func (f *adminFixture) do(request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *adminFixture) login(t *testing.T, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		t.Fatal(err)
	}
	return f.do(adminRequest(http.MethodPost, "/api/admin/login", string(body)))
}

func (f *adminFixture) adminCookie(t *testing.T) *http.Cookie {
	t.Helper()
	response := f.login(t, testAdminPassword)
	if response.Code != http.StatusNoContent {
		t.Fatalf("login = %d; want 204", response.Code)
	}
	return cookieNamed(t, response, auth.AdminCookieName)
}

func cookieNamed(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie; cookies = %#v", name, response.Result().Cookies())
	return nil
}

func TestAdminLoginRejectsWrongPasswordWithoutCookie(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	response := fx.login(t, "wrong password")
	if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"invalid_credentials\"}\n" {
		t.Fatalf("wrong password = %d %q; want 401 invalid_credentials", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 {
		t.Errorf("failed login set cookies %#v", response.Result().Cookies())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("failed login headers = %v; want no-store JSON", response.Header())
	}
}

func TestAdminLoginIssuesHardenedAdminCookieForCorrectPassword(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	response := fx.login(t, testAdminPassword)
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("login = %d %q; want empty 204", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q; want no-store", response.Header().Get("Cache-Control"))
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v; want only the admin session", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != auth.AdminCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Errorf("admin cookie attributes = %#v; want HttpOnly, Secure, SameSite=Strict, Path=/", cookie)
	}
	if cookie.MaxAge != int(12*time.Hour/time.Second) {
		t.Errorf("admin cookie max-age = %d; want 43200", cookie.MaxAge)
	}
}

func TestAdminLoginRejectsMalformedAndOversizedBodies(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	for _, test := range []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{"not JSON", "password=x", http.StatusBadRequest, "invalid_request"},
		{"wrong field type", `{"password":42}`, http.StatusBadRequest, "invalid_request"},
		{"unknown field", `{"password":"x","remember":true}`, http.StatusBadRequest, "invalid_request"},
		{"oversized body", `{"password":"` + strings.Repeat("x", 8192) + `"}`, http.StatusRequestEntityTooLarge, "request_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := fx.do(adminRequest(http.MethodPost, "/api/admin/login", test.body))
			if response.Code != test.wantStatus || response.Body.String() != "{\"error\":\""+test.wantError+"\"}\n" {
				t.Fatalf("login = %d %q; want %d %s", response.Code, response.Body.String(), test.wantStatus, test.wantError)
			}
			if len(response.Result().Cookies()) != 0 {
				t.Errorf("malformed login set cookies")
			}
		})
	}
}

func TestAdminRoutesRequireAdminSession(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	admin := fx.adminCookie(t)
	tampered := *admin
	tampered.Value = "A" + admin.Value[1:]
	if admin.Value[0] == 'A' {
		tampered.Value = "B" + admin.Value[1:]
	}
	spoofed := *guest
	spoofed.Name = auth.AdminCookieName

	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/status", ""},
		{http.MethodPost, "/api/admin/settings", `{"uploadsEnabled":false}`},
	} {
		for name, cookies := range map[string][]*http.Cookie{
			"no cookie":           nil,
			"guest cookie":        {guest},
			"tampered admin":      {&tampered},
			"guest renamed admin": {&spoofed},
		} {
			t.Run(route.method+" "+route.path+" "+name, func(t *testing.T) {
				response := fx.do(adminRequest(route.method, route.path, route.body, cookies...))
				if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"admin_session_required\"}\n" {
					t.Fatalf("response = %d %q; want 401 admin_session_required", response.Code, response.Body.String())
				}
				if response.Header().Get("Cache-Control") != "no-store" {
					t.Errorf("Cache-Control = %q; want no-store", response.Header().Get("Cache-Control"))
				}
			})
		}
	}
	if !fx.settings.UploadsEnabled() {
		t.Fatal("unauthenticated settings request changed the upload switch")
	}
}

func TestGuestRoutesRejectAdminCookie(t *testing.T) {
	fx := newAdminFixture(t, nil, &fakeGallery{})
	admin := fx.adminCookie(t)
	guestRenamed := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	guestRenamed.Name = auth.GuestCookieName

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/media"},
		{http.MethodGet, "/api/session"},
		{http.MethodPut, "/api/media/" + testMediaID},
		{http.MethodPost, "/api/media/" + testMediaID + "/derived?variant=t"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := fx.do(adminRequest(route.method, route.path, "", admin))
			if response.Code != http.StatusUnauthorized || response.Body.String() != "guest session required\n" {
				t.Fatalf("admin cookie on guest route = %d %q; want 401 guest session required", response.Code, response.Body.String())
			}
		})
	}

	// A guest cookie can never be presented as the admin cookie.
	relabelled := *guestRenamed
	relabelled.Name = auth.AdminCookieName
	response := fx.do(adminRequest(http.MethodGet, "/api/admin/status", "", &relabelled))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("guest value under admin cookie name = %d; want 401", response.Code)
	}
}

func TestAdminStatusReportsSwitchAndDiskPauseState(t *testing.T) {
	const gib = uint64(1 << 30)
	for _, test := range []struct {
		name      string
		free      uint64
		reserved  uint64
		minFree   uint64
		wantPause bool
	}{
		{"plenty of space", 10 * gib, 1 * gib, 5 * gib, false},
		{"free minus reserved exactly at threshold", 6 * gib, 1 * gib, 5 * gib, false},
		{"free minus reserved below threshold", 11 * gib / 2, 1 * gib, 5 * gib, true},
		{"reservations exceed free space", 1 * gib, 2 * gib, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fx := newAdminFixture(t, nil, nil)
			fx.disk.usage = upload.DiskUsage{FreeBytes: test.free, ReservedBytes: test.reserved, MinFreeBytes: test.minFree}
			cookie := fx.adminCookie(t)
			response := fx.do(adminRequest(http.MethodGet, "/api/admin/status", "", cookie))
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("status = %d %v; want 200 JSON", response.Code, response.Header())
			}
			var status map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatalf("status body %q is not JSON: %v", response.Body.String(), err)
			}
			want := map[string]any{
				"uploadsEnabled": true,
				"diskPaused":     test.wantPause,
				"freeBytes":      float64(test.free),
				"reservedBytes":  float64(test.reserved),
				"minFreeBytes":   float64(test.minFree),
			}
			if len(status) != len(want) {
				t.Fatalf("status keys = %v; want exactly %v", status, want)
			}
			for key, value := range want {
				if status[key] != value {
					t.Errorf("status[%s] = %v; want %v", key, status[key], value)
				}
			}
		})
	}
}

func TestAdminStatusFailsWhenDiskCannotBeMeasured(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	fx.disk.err = context.DeadlineExceeded
	response := fx.do(adminRequest(http.MethodGet, "/api/admin/status", "", fx.adminCookie(t)))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"status_unavailable\"}\n" {
		t.Fatalf("status with unreadable disk = %d %q; want 503 status_unavailable", response.Code, response.Body.String())
	}
}

func TestAdminSettingsTogglesUploadsAndReturnsStatus(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	cookie := fx.adminCookie(t)

	disable := fx.do(adminRequest(http.MethodPost, "/api/admin/settings", `{"uploadsEnabled":false}`, cookie))
	if disable.Code != http.StatusOK || disable.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("disable = %d %v; want 200 no-store", disable.Code, disable.Header())
	}
	var status map[string]any
	if err := json.Unmarshal(disable.Body.Bytes(), &status); err != nil || status["uploadsEnabled"] != false || status["minFreeBytes"] == nil {
		t.Fatalf("disable body = %q (%v); want status JSON with uploadsEnabled=false", disable.Body.String(), err)
	}
	if fx.settings.UploadsEnabled() {
		t.Fatal("settings endpoint did not update the switch")
	}
	stored := fx.do(adminRequest(http.MethodGet, "/api/admin/status", "", cookie))
	if !strings.Contains(stored.Body.String(), `"uploadsEnabled":false`) {
		t.Fatalf("status after disable = %q; want uploadsEnabled false", stored.Body.String())
	}

	enable := fx.do(adminRequest(http.MethodPost, "/api/admin/settings", `{"uploadsEnabled":true}`, cookie))
	if enable.Code != http.StatusOK || !strings.Contains(enable.Body.String(), `"uploadsEnabled":true`) {
		t.Fatalf("enable = %d %q; want 200 with uploadsEnabled true", enable.Code, enable.Body.String())
	}
}

func TestAdminSettingsRejectsInvalidBodies(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	cookie := fx.adminCookie(t)
	for _, body := range []string{``, `{}`, `{"uploadsEnabled":"no"}`, `{"uploadsEnabled":true,"other":1}`, `[true]`} {
		response := fx.do(adminRequest(http.MethodPost, "/api/admin/settings", body, cookie))
		if response.Code != http.StatusBadRequest || response.Body.String() != "{\"error\":\"invalid_request\"}\n" {
			t.Errorf("body %q = %d %q; want 400 invalid_request", body, response.Code, response.Body.String())
		}
	}
	if !fx.settings.UploadsEnabled() {
		t.Fatal("invalid settings body changed the switch")
	}
}

func TestAdminLogoutClearsAdminCookie(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	response := fx.do(adminRequest(http.MethodPost, "/api/admin/logout", ""))
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("logout = %d %q; want empty 204", response.Code, response.Body.String())
	}
	cleared := cookieNamed(t, response, auth.AdminCookieName)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("logout cookie = %#v; want cleared with negative max-age", cleared)
	}
}

func TestAdminLoginDelaysRetriesAfterFailuresAndResetsOnSuccess(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	attempt := func(password string, wantStatus int) {
		t.Helper()
		response := fx.login(t, password)
		if response.Code != wantStatus {
			t.Fatalf("login(%q) = %d; want %d", password, response.Code, wantStatus)
		}
	}

	attempt("wrong", http.StatusUnauthorized)
	if len(fx.clock.sleeps) != 0 {
		t.Fatalf("first failure delayed the next attempt: %v", fx.clock.sleeps)
	}
	attempt("wrong", http.StatusUnauthorized)
	attempt("wrong", http.StatusUnauthorized)
	attempt(testAdminPassword, http.StatusNoContent)
	// Each attempt after a failure waits first: 1s after one failure, 2s after two, 4s after three.
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(fx.clock.sleeps) != len(want) {
		t.Fatalf("delays = %v; want %v", fx.clock.sleeps, want)
	}
	for i := range want {
		if fx.clock.sleeps[i] != want[i] {
			t.Fatalf("delays = %v; want %v", fx.clock.sleeps, want)
		}
	}

	attempt("wrong", http.StatusUnauthorized)
	attempt("wrong", http.StatusUnauthorized)
	if got := fx.clock.sleeps[len(fx.clock.sleeps)-1]; got != time.Second {
		t.Fatalf("delay after success reset = %s; want 1s", got)
	}
}

func TestAdminLoginDelayIsCappedAtThirtySeconds(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	for i := 0; i < 12; i++ {
		fx.login(t, "wrong")
	}
	for _, delay := range fx.clock.sleeps {
		if delay > 30*time.Second {
			t.Fatalf("delay %s exceeds the 30s cap; delays = %v", delay, fx.clock.sleeps)
		}
	}
	if got := fx.clock.sleeps[len(fx.clock.sleeps)-1]; got != 30*time.Second {
		t.Fatalf("steady-state delay = %s; want 30s", got)
	}
}

// fakeGallery serves an empty confirmed page so gallery reads can be checked while uploads are paused.
type fakeGallery struct{}

func (fakeGallery) List(context.Context, string, int) (media.Page, error) {
	return media.Page{Items: []media.Item{}}, nil
}
