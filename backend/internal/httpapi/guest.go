package httpapi

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/PMG801/wedding/internal/auth"
)

func guestEntryHandler(eventToken string, sessionKey []byte) http.Handler {
	configuredToken, validToken := decodeEventToken(eventToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		providedToken, ok := decodeEventToken(r.PathValue("token"))
		if !validToken || !ok || subtle.ConstantTimeCompare(configuredToken, providedToken) != 1 {
			http.NotFound(w, r)
			return
		}

		http.SetCookie(w, auth.NewGuestCookie(sessionKey, time.Now()))
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
}

func decodeEventToken(token string) ([]byte, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(token)
	}
	return decoded, err == nil && len(decoded) == 32
}
