// Package auth implements guest and administrator session cookies, password checks and login throttling.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	GuestCookieName      = "guest_session"
	GuestSessionLifetime = 30 * 24 * time.Hour

	guestExpiryBytes = 8
	guestIDBytes     = 16
	// guestSessionDomain signs the legacy v1 payload: expiry only.
	guestSessionDomain = "boda-guest-session-v1:"
	// guestSessionDomainV2 signs the v2 payload: expiry followed by guest_id.
	guestSessionDomainV2 = "boda-guest-session-v2:"
)

// guestIDContextKey carries the guest_id of a validated guest request.
type guestIDContextKey struct{}

// GuestIDFromContext returns the anonymous guest_id that RequireGuestSession
// attached to the request. It is empty for requests without a guest session.
func GuestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(guestIDContextKey{}).(string)
	return id, ok && id != ""
}

// NewGuestCookie creates a signed guest session cookie with a fresh guest_id.
func NewGuestCookie(key []byte, now time.Time) *http.Cookie {
	return guestCookie(key, now, now.Add(GuestSessionLifetime).UTC(), newGuestID())
}

// RenewGuestCookie issues the cookie for a QR re-entry. A valid v2 cookie keeps
// its guest_id, so rescanning in the same browser does not create a new voter;
// any other request receives a fresh identity.
func RenewGuestCookie(key []byte, r *http.Request, now time.Time) *http.Cookie {
	guestID := newGuestID()
	if cookie, err := r.Cookie(GuestCookieName); err == nil {
		if session, ok := parseGuestSession(cookie.Value, key, now); ok && !session.legacy {
			guestID = session.guestID
		}
	}
	return guestCookie(key, now, now.Add(GuestSessionLifetime).UTC(), guestID)
}

// ValidGuestSession verifies the signature and expiration of a guest cookie in
// either the current v2 format or the legacy v1 format.
func ValidGuestSession(value string, key []byte, now time.Time) bool {
	_, ok := parseGuestSession(value, key, now)
	return ok
}

// RequireGuestSession wraps a protected handler with guest-cookie validation.
// It attaches the guest_id to the request context. A valid legacy v1 cookie is
// upgraded in the response to v2 with a fresh guest_id and the same expiry.
// Public endpoints should be registered outside this middleware.
func RequireGuestSession(key []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		cookie, err := r.Cookie(GuestCookieName)
		if err != nil {
			http.Error(w, "guest session required", http.StatusUnauthorized)
			return
		}
		session, ok := parseGuestSession(cookie.Value, key, now)
		if !ok {
			http.Error(w, "guest session required", http.StatusUnauthorized)
			return
		}
		guestID := session.guestID
		if session.legacy {
			guestID = newGuestID()
			http.SetCookie(w, guestCookie(key, now, session.expires, guestID))
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), guestIDContextKey{}, guestID)))
	})
}

// guestSession is a validated cookie payload. Legacy v1 cookies carry no guest_id.
type guestSession struct {
	expires time.Time
	guestID string
	legacy  bool
}

// guestCookie builds a v2 cookie with the given expiry and guest_id. MaxAge is
// measured from now so that an upgraded cookie keeps its original expiry.
func guestCookie(key []byte, now, expires time.Time, guestID string) *http.Cookie {
	return &http.Cookie{
		Name:     GuestCookieName,
		Value:    newGuestSignedValue(key, expires, guestID),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(expires.Sub(now).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// newGuestSignedValue encodes the v2 payload (8-byte expiry, 16-byte guest_id) followed by its HMAC.
func newGuestSignedValue(key []byte, expires time.Time, guestID string) string {
	raw, err := hex.DecodeString(guestID)
	if err != nil || len(raw) != guestIDBytes {
		panic("guest_id must be 16 random bytes encoded as hex")
	}
	payload := make([]byte, guestExpiryBytes, guestExpiryBytes+guestIDBytes)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	payload = append(payload, raw...)
	value := append(payload, sessionMAC(guestSessionDomainV2, key, payload)...)
	return base64.RawURLEncoding.EncodeToString(value)
}

// newSignedValue encodes an 8-byte expiry followed by its HMAC under a purpose-specific domain label.
func newSignedValue(domain string, key []byte, expires time.Time) string {
	payload := make([]byte, guestExpiryBytes)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	value := append(payload, sessionMAC(domain, key, payload)...)
	return base64.RawURLEncoding.EncodeToString(value)
}

// validSignedValue checks the MAC in constant time and then the expiry.
func validSignedValue(domain, value string, key []byte, now time.Time) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != guestExpiryBytes+sha256.Size {
		return false
	}
	payload, signature := decoded[:guestExpiryBytes], decoded[guestExpiryBytes:]
	if !hmac.Equal(signature, sessionMAC(domain, key, payload)) {
		return false
	}
	expires := int64(binary.BigEndian.Uint64(payload))
	return expires > now.Unix()
}

// parseGuestSession checks the MAC in constant time and then the expiry. It
// accepts v2 cookies and legacy v1 cookies; each has its own domain label.
func parseGuestSession(value string, key []byte, now time.Time) (guestSession, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return guestSession{}, false
	}
	v2Size := guestExpiryBytes + guestIDBytes + sha256.Size
	v1Size := guestExpiryBytes + sha256.Size
	var session guestSession
	switch len(decoded) {
	case v2Size:
		payload, signature := decoded[:guestExpiryBytes+guestIDBytes], decoded[guestExpiryBytes+guestIDBytes:]
		if !hmac.Equal(signature, sessionMAC(guestSessionDomainV2, key, payload)) {
			return guestSession{}, false
		}
		session.guestID = hex.EncodeToString(payload[guestExpiryBytes:])
	case v1Size:
		payload, signature := decoded[:guestExpiryBytes], decoded[guestExpiryBytes:]
		if !hmac.Equal(signature, sessionMAC(guestSessionDomain, key, payload)) {
			return guestSession{}, false
		}
		session.legacy = true
	default:
		return guestSession{}, false
	}
	session.expires = time.Unix(int64(binary.BigEndian.Uint64(decoded[:guestExpiryBytes])), 0).UTC()
	if !session.expires.After(now) {
		return guestSession{}, false
	}
	return session, true
}

// newGuestID returns 16 random bytes as 32 lowercase hex characters.
func newGuestID() string {
	raw := make([]byte, guestIDBytes)
	if _, err := rand.Read(raw); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(raw)
}

func sessionMAC(domain string, key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
