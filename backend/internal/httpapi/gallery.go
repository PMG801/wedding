package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
)

const (
	defaultGalleryLimit = 50
	maxGalleryLimit     = 100
)

type mediaLister interface {
	ListForGuest(ctx context.Context, guestID, cursor string, limit int) (media.Page, error)
	Top(ctx context.Context, guestID string) ([]media.Item, error)
}

func newGalleryHandler(gallery mediaLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gallery == nil {
			writeUploadError(w, http.StatusServiceUnavailable, "gallery_unavailable")
			return
		}
		limit, ok := parseGalleryLimit(r.URL.Query().Get("limit"))
		if !ok {
			writeUploadError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		guestID, _ := auth.GuestIDFromContext(r.Context())
		page, err := gallery.ListForGuest(r.Context(), guestID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			if errors.Is(err, media.ErrInvalidCursor) {
				writeUploadError(w, http.StatusBadRequest, "invalid_cursor")
				return
			}
			writeUploadError(w, http.StatusInternalServerError, "gallery_failed")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(page)
	})
}

// newTopHandler serves GET /api/media/top: the 50 most liked visible media in
// one response. Query parameters are ignored.
func newTopHandler(gallery mediaLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gallery == nil {
			writeUploadError(w, http.StatusServiceUnavailable, "gallery_unavailable")
			return
		}
		guestID, _ := auth.GuestIDFromContext(r.Context())
		items, err := gallery.Top(r.Context(), guestID)
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "gallery_failed")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Items []media.Item `json:"items"`
		}{Items: items})
	})
}

// parseGalleryLimit reads the optional page size shared by gallery listings.
func parseGalleryLimit(raw string) (int, bool) {
	if raw == "" {
		return defaultGalleryLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxGalleryLimit {
		return 0, false
	}
	return limit, true
}
