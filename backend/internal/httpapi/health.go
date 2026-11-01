package httpapi

import (
	"net/http"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// Option customizes NewHandler.
type Option func(*handlerOptions)

type handlerOptions struct {
	maxDerivativeBytes int64
	admin              *adminOptions
	throttle           *auth.LoginThrottle
	moderator          mediaModerator
	hiddenMedia        hiddenMediaSource
	likes              likeStore
	pinned             pinnedStore
	resumable          *upload.Resumable
}

// WithResumable mounts the tus endpoint for original media at /api/uploads/ (ADR-0014).
func WithResumable(resumable *upload.Resumable) Option {
	return func(o *handlerOptions) { o.resumable = resumable }
}

// WithMaxDerivativeBytes sets the largest accepted derivative; when unset,
// derivatives keep the photo limit.
func WithMaxDerivativeBytes(limit int64) Option {
	return func(o *handlerOptions) { o.maxDerivativeBytes = limit }
}

// NewHandler builds the HTTP handler for public and guest-session routes.
func NewHandler(eventToken string, sessionKey []byte, photos derivativePhotoUploader, maxConcurrent int, idleTimeout time.Duration, maxPhotoBytes int64, gallery mediaLister, opts ...Option) http.Handler {
	var options handlerOptions
	for _, opt := range opts {
		opt(&options)
	}
	mux := http.NewServeMux()
	capacity := newUploadCapacity(maxConcurrent)
	maxDerivative := maxPhotoBytes
	if options.maxDerivativeBytes > 0 {
		maxDerivative = options.maxDerivativeBytes
	}
	var switches uploadSwitch
	if options.admin != nil {
		switches = options.admin.switches
	}
	mux.Handle("GET /e/{token}", guestEntryHandler(eventToken, sessionKey))
	if options.resumable != nil {
		registerResumableRoutes(mux, sessionKey, switches, capacity, options.resumable)
	}
	mux.Handle("POST /api/media/{id}/derived", auth.RequireGuestSession(sessionKey, uploadsSwitchGate(switches, newDerivativeUploadHandler(photos, capacity, maxDerivative, idleTimeout))))
	registerAdminRoutes(mux, sessionKey, options)
	registerLikeRoutes(mux, sessionKey, options.likes)
	registerPinnedRoutes(mux, sessionKey, options.pinned)
	mux.Handle("GET /api/media", auth.RequireGuestSession(sessionKey, newGalleryHandler(gallery)))
	mux.Handle("GET /api/media/top", auth.RequireGuestSession(sessionKey, newTopHandler(gallery)))
	mux.Handle("GET /api/session", auth.RequireGuestSession(sessionKey, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})))
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
