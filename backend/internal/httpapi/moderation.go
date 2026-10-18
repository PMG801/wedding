package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// maxModerationBodyBytes bounds PATCH /api/admin/media/{id} bodies.
const maxModerationBodyBytes = 1 << 10

// mediaModerator hides and shows media; *upload.Store implements it. There is
// deliberately no delete operation: hiding is the only way to withdraw media.
type mediaModerator interface {
	SetVisibility(ctx context.Context, id, visibility string) (upload.Media, error)
}

// WithModeration enables PATCH /api/admin/media/{id}.
func WithModeration(moderator mediaModerator) Option {
	return func(o *handlerOptions) { o.moderator = moderator }
}

func registerModerationRoutes(mux *http.ServeMux, sessionKey []byte, moderator mediaModerator) {
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeUploadError(w, http.StatusServiceUnavailable, "moderation_unavailable")
	}))
	if moderator != nil {
		handler = moderationHandler(moderator)
	}
	mux.Handle("PATCH /api/admin/media/{id}", noStore(auth.RequireAdminSession(sessionKey, handler)))
}

func moderationHandler(moderator mediaModerator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Visibility *string `json:"visibility"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxModerationBodyBytes)
		err := decodeStrictJSON(r, &body)
		if err != nil || body.Visibility == nil || (*body.Visibility != "hidden" && *body.Visibility != "visible") {
			writeUploadError(w, http.StatusBadRequest, "invalid_visibility")
			return
		}
		media, err := moderator.SetVisibility(r.Context(), r.PathValue("id"), *body.Visibility)
		switch {
		case errors.Is(err, upload.ErrInvalidVisibility):
			writeUploadError(w, http.StatusBadRequest, "invalid_visibility")
		case errors.Is(err, upload.ErrMediaNotFound):
			writeUploadError(w, http.StatusNotFound, "media_not_found")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "moderation_failed")
		default:
			writeJSON(w, http.StatusOK, struct {
				ID         string `json:"id"`
				Visibility string `json:"visibility"`
			}{media.ID, media.Visibility})
		}
	})
}
