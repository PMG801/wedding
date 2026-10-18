package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
)

func TestGuestSessionCheckAuthorizesOnlyValidGuestCookies(t *testing.T) {
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, nil)
	for _, test := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"missing cookie", nil, http.StatusUnauthorized},
		{"forged cookie", &http.Cookie{Name: "guest_session", Value: "forged"}, http.StatusUnauthorized},
		{"expired cookie", auth.NewGuestCookie([]byte(testSessionKey), time.Now().Add(-31*24*time.Hour)), http.StatusUnauthorized},
		{"valid cookie", auth.NewGuestCookie([]byte(testSessionKey), time.Now()), http.StatusNoContent},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		if test.cookie != nil {
			request.AddCookie(test.cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s: GET /api/session status = %d; want %d", test.name, response.Code, test.want)
		}
		if test.want == http.StatusNoContent && response.Body.Len() != 0 {
			t.Fatalf("%s: GET /api/session body = %q; want empty", test.name, response.Body.String())
		}
	}
}

func TestLegacyGuestCookieStillPassesSessionCheckAndIsUpgraded(t *testing.T) {
	key := []byte(testSessionKey)
	expires := time.Now().Add(5 * 24 * time.Hour).UTC().Truncate(time.Second)
	legacy := legacyGuestCookieForTest(key, expires)

	handler := NewHandler(testEventToken, key, nil, 1, time.Second, 1024, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(legacy)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("legacy cookie session = %d headers %v; want 204 no-store", response.Code, response.Header())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "guest_session" || !cookies[0].Expires.Equal(expires) {
		t.Fatalf("Set-Cookie = %#v; want one upgraded guest cookie with the original expiry", cookies)
	}
}

// legacyGuestCookieForTest builds a v1 guest cookie, the format issued before guest_id.
func legacyGuestCookieForTest(key []byte, expires time.Time) *http.Cookie {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("boda-guest-session-v1:"))
	_, _ = mac.Write(payload)
	return &http.Cookie{Name: "guest_session", Value: base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))}
}
