package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuestQRExchangeRedirectsAndDoesNotExposeToken(t *testing.T) {
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	handler := NewHandler(token, []byte("0123456789abcdef0123456789abcdef"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/e/"+token, nil))

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("response = %d location %q; want redirect to /", response.Code, response.Header().Get("Location"))
	}
	if strings.Contains(response.Header().Get("Location"), token) || strings.Contains(response.Body.String(), token) {
		t.Fatal("response exposed the event token")
	}
	cookie := response.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != "guest_session" {
		t.Fatalf("cookies = %#v; want one guest session cookie", cookie)
	}
	if !cookie[0].HttpOnly || !cookie[0].Secure || cookie[0].SameSite != http.SameSiteLaxMode {
		t.Errorf("guest cookie is missing required security attributes: %#v", cookie[0])
	}
}

func TestGuestQRExchangeRejectsWrongTokenAndHealthStaysPublic(t *testing.T) {
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	handler := NewHandler(token, []byte("0123456789abcdef0123456789abcdef"))

	wrongToken := httptest.NewRecorder()
	handler.ServeHTTP(wrongToken, httptest.NewRequest(http.MethodGet, "/e/BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", nil))
	if wrongToken.Code != http.StatusNotFound || len(wrongToken.Result().Cookies()) != 0 {
		t.Errorf("wrong token response = %d, cookies %#v; want 404 without cookie", wrongToken.Code, wrongToken.Result().Cookies())
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/e/"+token, nil))
	if head.Code != http.StatusMethodNotAllowed || len(head.Result().Cookies()) != 0 {
		t.Errorf("HEAD exchange response = %d, cookies %#v; want 405 without cookie", head.Code, head.Result().Cookies())
	}

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK || health.Body.String() != `{"status":"ok"}` {
		t.Errorf("health response = %d %q; want public health JSON", health.Code, health.Body.String())
	}
}
