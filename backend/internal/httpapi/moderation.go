package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
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
	handler := http.Handler(moderationUnavailable)
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

// hiddenMediaSource lists hidden media and opens their private previews;
// *media.Gallery implements it.
type hiddenMediaSource interface {
	ListHidden(ctx context.Context, cursor string, limit int) (media.Page, error)
	OpenHiddenPreview(ctx context.Context, id, variant string) (*os.File, error)
}

// WithHiddenMedia enables the administrator listing and previews of hidden media.
func WithHiddenMedia(source hiddenMediaSource) Option {
	return func(o *handlerOptions) { o.hiddenMedia = source }
}

func registerHiddenMediaRoutes(mux *http.ServeMux, sessionKey []byte, source hiddenMediaSource) {
	listing, preview := http.Handler(moderationUnavailable), http.Handler(moderationUnavailable)
	if source != nil {
		listing, preview = hiddenListingHandler(source), hiddenPreviewHandler(source)
	}
	mux.Handle("GET /api/admin/media", noStore(auth.RequireAdminSession(sessionKey, listing)))
	mux.Handle("GET /api/admin/media/{id}/preview/{variant}", noStore(auth.RequireAdminSession(sessionKey, preview)))
}

var moderationUnavailable = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	writeUploadError(w, http.StatusServiceUnavailable, "moderation_unavailable")
})

func hiddenListingHandler(source hiddenMediaSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("visibility") != "hidden" {
			writeUploadError(w, http.StatusBadRequest, "invalid_visibility")
			return
		}
		limit, ok := parseGalleryLimit(query.Get("limit"))
		if !ok {
			writeUploadError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		page, err := source.ListHidden(r.Context(), query.Get("cursor"), limit)
		switch {
		case errors.Is(err, media.ErrInvalidCursor):
			writeUploadError(w, http.StatusBadRequest, "invalid_cursor")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "gallery_failed")
		default:
			writeJSON(w, http.StatusOK, adminHiddenPage(page))
		}
	})
}

// adminHiddenItem is the administrator view of a hidden medium. Likes belong to
// the guest gallery, so this listing keeps its original shape.
type adminHiddenItem struct {
	ID          string  `json:"id"`
	MIME        string  `json:"mime"`
	ConfirmedAt int64   `json:"confirmed_at"`
	OriginalURL *string `json:"original_url"`
	ThumbURL    *string `json:"thumb_url"`
	DetailURL   *string `json:"detail_url"`
}

type adminHiddenPageBody struct {
	Items      []adminHiddenItem `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

func adminHiddenPage(page media.Page) adminHiddenPageBody {
	items := make([]adminHiddenItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, adminHiddenItem{
			ID: item.ID, MIME: item.MIME, ConfirmedAt: item.ConfirmedAt,
			OriginalURL: item.OriginalURL, ThumbURL: item.ThumbURL, DetailURL: item.DetailURL,
		})
	}
	return adminHiddenPageBody{Items: items, NextCursor: page.NextCursor}
}

func hiddenPreviewHandler(source hiddenMediaSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, err := source.OpenHiddenPreview(r.Context(), r.PathValue("id"), r.PathValue("variant"))
		if errors.Is(err, media.ErrPreviewNotFound) {
			writeUploadError(w, http.StatusNotFound, "preview_not_found")
			return
		}
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "preview_failed")
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "preview_failed")
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}
