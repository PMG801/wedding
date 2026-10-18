package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminCookieIsSeparateSignedAndHardened(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cookie := NewAdminCookie(key, now)

	if cookie.Name != AdminCookieName || cookie.Name == GuestCookieName || cookie.Value == "" {
		t.Fatalf("cookie = %#v; want a named admin cookie distinct from the guest cookie", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Errorf("admin cookie security attributes = %#v; want HttpOnly, Secure, SameSite=Strict, Path=/", cookie)
	}
	if AdminSessionLifetime != 12*time.Hour {
		t.Errorf("AdminSessionLifetime = %s; want 12h", AdminSessionLifetime)
	}
	if cookie.Expires.Sub(now) != AdminSessionLifetime || cookie.MaxAge != int(AdminSessionLifetime.Seconds()) {
		t.Errorf("admin cookie lifetime = expires %v, max-age %d", cookie.Expires, cookie.MaxAge)
	}
	if !ValidAdminSession(cookie.Value, key, now.Add(AdminSessionLifetime-time.Second)) {
		t.Fatal("new admin cookie was not accepted before expiration")
	}
	if ValidAdminSession(cookie.Value, key, now.Add(AdminSessionLifetime)) {
		t.Fatal("admin cookie was accepted at expiration")
	}
}

func TestAdminAndGuestSessionsAreDomainSeparated(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now()
	guest := NewGuestCookie(key, now).Value
	admin := NewAdminCookie(key, now).Value

	if ValidAdminSession(guest, key, now) {
		t.Error("guest session value was accepted as an admin session")
	}
	if ValidGuestSession(admin, key, now) {
		t.Error("admin session value was accepted as a guest session")
	}
	if ValidAdminSession(admin, []byte("fedcba9876543210fedcba9876543210"), now) {
		t.Error("admin session was accepted under a different key")
	}
	replacement := "A"
	if admin[0] == 'A' {
		replacement = "B"
	}
	if ValidAdminSession(replacement+admin[1:], key, now) {
		t.Error("tampered admin session was accepted")
	}
}

func TestExpiredAdminCookieClearsSession(t *testing.T) {
	cleared := ExpiredAdminCookie()
	if cleared.Name != AdminCookieName || cleared.Value != "" || cleared.MaxAge >= 0 || !cleared.Expires.Before(time.Now()) {
		t.Errorf("cleared cookie = %#v; want an expired empty %s cookie", cleared, AdminCookieName)
	}
	if !cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteStrictMode || cleared.Path != "/" {
		t.Errorf("cleared cookie lost attributes: %#v", cleared)
	}
}

func TestRequireAdminSessionReturnsJSONUnauthorizedWithoutAdminCookie(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	protected := RequireAdminSession(key, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	guestOnly := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	guestOnly.AddCookie(NewGuestCookie(key, time.Now()))
	for name, request := range map[string]*http.Request{
		"no cookie":    httptest.NewRequest(http.MethodGet, "/api/admin/status", nil),
		"guest cookie": guestOnly,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			protected.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"admin_session_required\"}\n" {
				t.Fatalf("response = %d %q; want 401 admin_session_required", response.Code, response.Body.String())
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
				t.Errorf("Content-Type = %q; want JSON", response.Header().Get("Content-Type"))
			}
		})
	}

	authorized := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	authorized.AddCookie(NewAdminCookie(key, time.Now()))
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, authorized)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid admin session status = %d; want 204", response.Code)
	}
}
