package httpapi

import (
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
