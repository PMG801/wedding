// Package auth implements guest-session cookies and middleware.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"time"
)

const (
	GuestCookieName         = "guest_session"
	GuestSessionLifetime    = 30 * 24 * time.Hour
	guestSessionPayloadSize = 8
)

// NewGuestCookie creates a signed, opaque guest session cookie.
func NewGuestCookie(key []byte, now time.Time) *http.Cookie {
	expires := now.Add(GuestSessionLifetime).UTC()
	payload := make([]byte, guestSessionPayloadSize)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	mac := guestSessionMAC(key, payload)
	value := append(payload, mac...)
	return &http.Cookie{
		Name:     GuestCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(value),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(GuestSessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// ValidGuestSession verifies the signature and expiration of a guest cookie.
func ValidGuestSession(value string, key []byte, now time.Time) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != guestSessionPayloadSize+sha256.Size {
		return false
	}
	payload, signature := decoded[:guestSessionPayloadSize], decoded[guestSessionPayloadSize:]
	if !hmac.Equal(signature, guestSessionMAC(key, payload)) {
		return false
	}
	expires := int64(binary.BigEndian.Uint64(payload))
	return expires > now.Unix()
}

// RequireGuestSession wraps a protected handler with guest-cookie validation.
// Public endpoints should be registered outside this middleware.
func RequireGuestSession(key []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(GuestCookieName)
		if err != nil || !ValidGuestSession(cookie.Value, key, time.Now()) {
			http.Error(w, "guest session required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func guestSessionMAC(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("boda-guest-session-v1:"))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
