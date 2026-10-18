package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/upload"
)

// likeStore records anonymous «me gusta» votes; *media.Likes implements it.
type likeStore interface {
	Like(ctx context.Context, guestID, mediaID string) (media.LikeState, error)
	Unlike(ctx context.Context, guestID, mediaID string) (media.LikeState, error)
}

// WithLikes enables PUT and DELETE /api/media/{id}/like. Likes are not gated
// by the uploads switch.
func WithLikes(store likeStore) Option {
	return func(o *handlerOptions) { o.likes = store }
}

func registerLikeRoutes(mux *http.ServeMux, sessionKey []byte, store likeStore) {
	put, remove := http.Handler(likesUnavailable), http.Handler(likesUnavailable)
	if store != nil {
		put, remove = likeHandler(store, true), likeHandler(store, false)
	}
	mux.Handle("PUT /api/media/{id}/like", noStore(auth.RequireGuestSession(sessionKey, put)))
	mux.Handle("DELETE /api/media/{id}/like", noStore(auth.RequireGuestSession(sessionKey, remove)))
}

var likesUnavailable = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	writeUploadError(w, http.StatusServiceUnavailable, "likes_unavailable")
})

// likeResponse is the final state of one medium for the caller's identity.
type likeResponse struct {
	ID        string `json:"id"`
	LikeCount int64  `json:"like_count"`
	LikedByMe bool   `json:"liked_by_me"`
}

func likeHandler(store likeStore, like bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guestID, ok := auth.GuestIDFromContext(r.Context())
		if !ok {
			http.Error(w, "guest session required", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		if !upload.ValidID(id) {
			writeUploadError(w, http.StatusBadRequest, "invalid_media_id")
			return
		}
		var state media.LikeState
		var err error
		if like {
			state, err = store.Like(r.Context(), guestID, id)
		} else {
			state, err = store.Unlike(r.Context(), guestID, id)
		}
		switch {
		case errors.Is(err, media.ErrMediaNotFound):
			writeUploadError(w, http.StatusNotFound, "media_not_found")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "like_failed")
		default:
			writeJSON(w, http.StatusOK, likeResponse{ID: state.ID, LikeCount: state.LikeCount, LikedByMe: state.LikedByMe})
		}
	})
}
