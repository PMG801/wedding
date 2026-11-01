package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// registerResumableRoutes mounts the tus 1.0.0 endpoint (ADR-0014). Every route
// needs a guest session; only creation and PATCH obey the upload switch and the
// shared capacity, and reads and DELETE keep working while uploads are paused.
func registerResumableRoutes(mux *http.ServeMux, sessionKey []byte, switches uploadSwitch, capacity chan struct{}, resumable *upload.Resumable) {
	mux.Handle("POST /api/uploads/{$}", auth.RequireGuestSession(sessionKey, uploadsSwitchGate(switches, resumable)))
	mux.Handle("PATCH /api/uploads/{id}", auth.RequireGuestSession(sessionKey, uploadsSwitchGate(switches, newResumablePatchHandler(resumable, capacity))))
	mux.Handle("HEAD /api/uploads/{id}", auth.RequireGuestSession(sessionKey, newResumableHeadHandler(resumable)))
	mux.Handle("DELETE /api/uploads/{id}", auth.RequireGuestSession(sessionKey, newResumableDeleteHandler(resumable)))
}

// newResumablePatchHandler takes one shared upload slot per PATCH and refuses a
// full capacity before reading the body.
func newResumablePatchHandler(resumable *upload.Resumable, capacity chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upload.ValidID(r.PathValue("id")) {
			writeUploadError(w, http.StatusNotFound, "upload_not_found")
			return
		}
		release, ok := acquireUploadSlot(capacity)
		if !ok {
			w.Header().Set("Retry-After", "1")
			writeUploadError(w, http.StatusServiceUnavailable, "upload_capacity_reached")
			return
		}
		defer release()
		resumable.ServeHTTP(w, r)
	})
}

func newResumableHeadHandler(resumable *upload.Resumable) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !upload.ValidID(id) {
			writeUploadError(w, http.StatusNotFound, "upload_not_found")
			return
		}
		status, err := resumable.Head(r.Context(), id)
		if errors.Is(err, upload.ErrResumableNotFound) {
			writeUploadError(w, http.StatusNotFound, "upload_not_found")
			return
		}
		if err != nil {
			code, message := upload.PublishErrorStatus(err)
			writeUploadError(w, code, message)
			return
		}
		header := w.Header()
		header.Set("Tus-Resumable", "1.0.0")
		header.Set("Cache-Control", "no-store")
		header.Set("Upload-Offset", strconv.FormatInt(status.Offset, 10))
		header.Set("Upload-Length", strconv.FormatInt(status.Length, 10))
		w.WriteHeader(http.StatusOK)
	})
}

func newResumableDeleteHandler(resumable *upload.Resumable) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upload.ValidID(r.PathValue("id")) {
			writeUploadError(w, http.StatusNotFound, "upload_not_found")
			return
		}
		resumable.ServeHTTP(w, r)
	})
}
