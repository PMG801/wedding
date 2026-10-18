package auth

import (
	"net/http"
	"time"
)

const (
	// AdminCookieName is the administrator session cookie, deliberately distinct from the guest cookie.
	AdminCookieName = "admin_session"
	// AdminSessionLifetime bounds an administrator session; logout cannot revoke a stateless cookie early.
	AdminSessionLifetime = 12 * time.Hour

	adminSessionDomain = "boda-admin-session-v1:"
)

// NewAdminCookie creates a signed administrator session cookie. It shares the
// session key with guest cookies but its MAC uses a separate domain label, so
// one purpose can never be replayed as the other.
func NewAdminCookie(key []byte, now time.Time) *http.Cookie {
	expires := now.Add(AdminSessionLifetime).UTC()
	return &http.Cookie{
		Name:     AdminCookieName,
		Value:    newSignedValue(adminSessionDomain, key, expires),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(AdminSessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

// ExpiredAdminCookie returns a cookie that makes the browser discard the administrator session.
func ExpiredAdminCookie() *http.Cookie {
	return &http.Cookie{
		Name:     AdminCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

// ValidAdminSession verifies the signature and expiration of an administrator cookie.
func ValidAdminSession(value string, key []byte, now time.Time) bool {
	return validSignedValue(adminSessionDomain, value, key, now)
}

// RequireAdminSession wraps an administrator handler. Requests without a valid
// administrator cookie receive a JSON 401 and never reach next.
func RequireAdminSession(key []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(AdminCookieName)
		if err != nil || !ValidAdminSession(cookie.Value, key, time.Now()) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("{\"error\":\"admin_session_required\"}\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
