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

func TestOldFormatGuestCookieFailsSessionCheck(t *testing.T) {
	key := []byte(testSessionKey)
	old := legacyGuestCookieForTest(key, time.Now().Add(5*24*time.Hour))

	handler := NewHandler(testEventToken, key, nil, 1, time.Second, 1024, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(old)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || response.Body.String() != "guest session required\n" {
		t.Fatalf("old cookie session = %d %q; want 401 text", response.Code, response.Body.String())
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("old cookie got Set-Cookie %v", cookies)
	}
}

// legacyGuestCookieForTest builds a correctly signed guest cookie in the old format without guest_id.
func legacyGuestCookieForTest(key []byte, expires time.Time) *http.Cookie {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("boda-guest-session-v1:"))
	_, _ = mac.Write(payload)
	return &http.Cookie{Name: "guest_session", Value: base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))}
}
