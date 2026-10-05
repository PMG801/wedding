package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGuestCookieIsSignedAndHasRequiredAttributes(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cookie := NewGuestCookie(key, now)

	if cookie.Name != GuestCookieName || cookie.Value == "" {
		t.Fatalf("cookie = %#v; want named signed guest cookie", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie security attributes = %#v", cookie)
	}
	if cookie.Expires.Sub(now) != GuestSessionLifetime || cookie.MaxAge != int(GuestSessionLifetime.Seconds()) {
		t.Errorf("cookie lifetime = expires %v, max-age %d", cookie.Expires, cookie.MaxAge)
	}
	if !ValidGuestSession(cookie.Value, key, now.Add(GuestSessionLifetime-time.Second)) {
		t.Fatal("new guest cookie was not accepted before expiration")
	}
	if ValidGuestSession(cookie.Value, key, now.Add(GuestSessionLifetime)) {
		t.Fatal("guest cookie was accepted at expiration")
	}
}

func TestRequireGuestSessionProtectsHandler(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	protected := RequireGuestSession(key, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthorized := httptest.NewRecorder()
	protected.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/media", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d; want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	cookie := NewGuestCookie(key, time.Now())
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	authorizedRequest.AddCookie(cookie)
	authorized := httptest.NewRecorder()
	protected.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("valid session status = %d; want %d", authorized.Code, http.StatusNoContent)
	}

	tampered := *cookie
	replacement := "A"
	if cookie.Value[0] == 'A' {
		replacement = "B"
	}
	tampered.Value = replacement + cookie.Value[1:]
	tamperedRequest := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	tamperedRequest.AddCookie(&tampered)
	tamperedResponse := httptest.NewRecorder()
	protected.ServeHTTP(tamperedResponse, tamperedRequest)
	if tamperedResponse.Code != http.StatusUnauthorized {
		t.Errorf("tampered session status = %d; want %d", tamperedResponse.Code, http.StatusUnauthorized)
	}
}
