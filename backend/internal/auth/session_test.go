package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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

// legacyGuestCookieValue encodes the v1 format that cookies issued before
// guest_id carry: 8-byte expiry plus an HMAC under the v1 domain label.
func legacyGuestCookieValue(key []byte, expires time.Time) string {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("boda-guest-session-v1:"))
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}

// serveGuestRequest runs cookie through RequireGuestSession and returns the
// response plus the guest_id the protected handler observed (empty on 401).
func serveGuestRequest(t *testing.T, key []byte, cookie *http.Cookie) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var seen string
	protected := RequireGuestSession(key, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := GuestIDFromContext(r.Context())
		if !ok {
			t.Errorf("guest_id missing from request context")
		}
		seen = id
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	return response, seen
}

func TestGuestCookieV2CarriesStableRandomGuestID(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now()
	cookie := NewGuestCookie(key, now)

	first, firstID := serveGuestRequest(t, key, cookie)
	again, againID := serveGuestRequest(t, key, cookie)
	_, otherID := serveGuestRequest(t, key, NewGuestCookie(key, now))

	if first.Code != http.StatusNoContent || again.Code != http.StatusNoContent {
		t.Fatalf("status = %d then %d; want 204 twice", first.Code, again.Code)
	}
	if len(first.Result().Cookies()) != 0 {
		t.Errorf("v2 cookie response set cookies %v; want none", first.Result().Cookies())
	}
	if !validGuestID(firstID) {
		t.Fatalf("guest_id = %q; want 32 lowercase hex characters", firstID)
	}
	if firstID != againID {
		t.Errorf("same cookie guest_id changed from %q to %q", firstID, againID)
	}
	if otherID == firstID {
		t.Errorf("distinct cookies share guest_id %q", firstID)
	}
}

func TestOldFormatCookieWithoutGuestIDIsRejected(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	old := &http.Cookie{Name: GuestCookieName, Value: legacyGuestCookieValue(key, now.Add(10*24*time.Hour))}

	if ValidGuestSession(old.Value, key, now) {
		t.Fatal("cookie without guest_id was accepted")
	}
	response, guestID := serveGuestRequest(t, key, old)
	if response.Code != http.StatusUnauthorized || response.Body.String() != "guest session required\n" {
		t.Fatalf("old cookie response = %d %q; want 401 text", response.Code, response.Body.String())
	}
	if guestID != "" || len(response.Result().Cookies()) != 0 {
		t.Errorf("old cookie reached the handler (guest_id %q) or got Set-Cookie %v", guestID, response.Result().Cookies())
	}
}

func TestTamperedGuestCookieIsRejectedWithoutUpgrade(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cookie := NewGuestCookie(key, time.Now())
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	raw[8] ^= 0x01 // flip one bit of guest_id
	tampered := &http.Cookie{Name: GuestCookieName, Value: base64.RawURLEncoding.EncodeToString(raw)}
	response, _ := serveGuestRequest(t, key, tampered)
	if response.Code != http.StatusUnauthorized || response.Body.String() != "guest session required\n" {
		t.Fatalf("tampered v2 response = %d %q; want 401 text", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 {
		t.Errorf("tampered cookie got Set-Cookie %v", response.Result().Cookies())
	}
}

func TestRenewGuestCookieKeepsGuestIDOfValidV2Cookie(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	existing := NewGuestCookie(key, now.Add(-20*24*time.Hour))
	request := httptest.NewRequest(http.MethodGet, "/e/token", nil)
	request.AddCookie(existing)
	_, existingID := serveGuestRequest(t, key, existing)

	renewed := RenewGuestCookie(key, request, now)
	if !ValidGuestSession(renewed.Value, key, now) {
		t.Fatal("renewed cookie is invalid")
	}
	if renewed.Expires.Unix() != now.Add(GuestSessionLifetime).Unix() {
		t.Errorf("renewed expiry = %v; want 30 days from now", renewed.Expires)
	}
	_, renewedID := serveGuestRequest(t, key, renewed)
	if renewedID != existingID {
		t.Errorf("renewed guest_id = %q; want existing %q", renewedID, existingID)
	}

	fresh := httptest.NewRequest(http.MethodGet, "/e/token", nil)
	freshCookie := RenewGuestCookie(key, fresh, now)
	_, freshID := serveGuestRequest(t, key, freshCookie)
	if !validGuestID(freshID) || freshID == existingID {
		t.Errorf("cookieless entry guest_id = %q; want a new identity", freshID)
	}

	legacyRequest := httptest.NewRequest(http.MethodGet, "/e/token", nil)
	legacyRequest.AddCookie(&http.Cookie{Name: GuestCookieName, Value: legacyGuestCookieValue(key, now.Add(time.Hour))})
	legacyRenewed := RenewGuestCookie(key, legacyRequest, now)
	_, legacyID := serveGuestRequest(t, key, legacyRenewed)
	if !validGuestID(legacyID) {
		t.Errorf("entry with an old cookie guest_id = %q; want a new identity", legacyID)
	}
}

func validGuestID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
