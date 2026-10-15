package httpapi

import (
	"net/http"
	"time"

	"github.com/PMG801/wedding/internal/auth"
)

// NewHandler builds the HTTP handler for public and guest-session routes.
func NewHandler(eventToken string, sessionKey []byte, photos photoUploader, maxConcurrent int, idleTimeout time.Duration, maxPhotoBytes int64, gallery mediaLister) http.Handler {
	mux := http.NewServeMux()
	capacity := newUploadCapacity(maxConcurrent)
	mux.Handle("GET /e/{token}", guestEntryHandler(eventToken, sessionKey))
	mux.Handle("PUT /api/media/{id}", auth.RequireGuestSession(sessionKey, newPhotoUploadHandler(photos, capacity, maxPhotoBytes, idleTimeout)))
	mux.Handle("POST /api/media/{id}/derived", auth.RequireGuestSession(sessionKey, newDerivativeUploadHandler(photos, capacity, maxPhotoBytes, idleTimeout)))
	mux.Handle("GET /api/media", auth.RequireGuestSession(sessionKey, newGalleryHandler(gallery)))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// ServeMux treats HEAD as matching GET, so reject it explicitly.
	mux.HandleFunc("HEAD /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	return mux
}
