package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
)

// carouselReader serves the story strip and the period albums (ADR-0014).
type carouselReader interface {
	Strip(ctx context.Context, guestID string) (media.Strip, error)
	Album(ctx context.Context, guestID string, periodID int, cursor string, limit int) (media.Page, error)
}

// WithCarousel enables GET /api/carousels and GET /api/carousels/{id}.
func WithCarousel(reader carouselReader) Option {
	return func(o *handlerOptions) { o.carousel = reader }
}

// registerCarouselRoutes mounts the guest strip and album. Without a reader the
// routes answer 503, like the other optional services.
func registerCarouselRoutes(mux *http.ServeMux, sessionKey []byte, reader carouselReader) {
	strip, album := http.Handler(http.HandlerFunc(carouselUnavailable)), http.Handler(http.HandlerFunc(carouselUnavailable))
	if reader != nil {
		strip = carouselStripHandler(reader)
		album = carouselAlbumHandler(reader)
	}
	mux.Handle("GET /api/carousels", noStore(auth.RequireGuestSession(sessionKey, strip)))
	mux.Handle("GET /api/carousels/{id}", noStore(auth.RequireGuestSession(sessionKey, album)))
}

func carouselUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeUploadError(w, http.StatusServiceUnavailable, "carousels_unavailable")
}

func carouselStripHandler(reader carouselReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guestID, _ := auth.GuestIDFromContext(r.Context())
		strip, err := reader.Strip(r.Context(), guestID)
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "carousel_failed")
			return
		}
		writeJSON(w, http.StatusOK, strip)
	})
}

func carouselAlbumHandler(reader carouselReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.PathValue("id")
		periodID, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(periodID) != raw {
			writeUploadError(w, http.StatusNotFound, "period_not_found")
			return
		}
		limit, ok := parseGalleryLimit(r.URL.Query().Get("limit"))
		if !ok {
			writeUploadError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		guestID, _ := auth.GuestIDFromContext(r.Context())
		page, err := reader.Album(r.Context(), guestID, periodID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			switch {
			case errors.Is(err, media.ErrPeriodNotFound):
				writeUploadError(w, http.StatusNotFound, "period_not_found")
			case errors.Is(err, media.ErrInvalidCursor):
				writeUploadError(w, http.StatusBadRequest, "invalid_cursor")
			default:
				writeUploadError(w, http.StatusInternalServerError, "carousel_failed")
			}
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
}
