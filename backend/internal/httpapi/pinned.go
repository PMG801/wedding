package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/pinned"
)

// maxPinnedBodyBytes bounds the JSON bodies of the pinned-entry routes (ADR-0013, D7).
const maxPinnedBodyBytes = 8 << 10

// pinnedStore manages the pinned entries; *pinned.Store implements it.
type pinnedStore interface {
	List(ctx context.Context) ([]pinned.Item, error)
	Create(ctx context.Context, in pinned.Input) (pinned.Item, error)
	Update(ctx context.Context, id string, in pinned.Input) (pinned.Item, error)
	Delete(ctx context.Context, id string) error
	Reorder(ctx context.Context, ids []string) ([]pinned.Item, error)
}

// WithPinned enables the pinned-entry routes (ADR-0013).
func WithPinned(store pinnedStore) Option {
	return func(o *handlerOptions) { o.pinned = store }
}

// registerPinnedRoutes mounts the guest read and the administrator CRUD. The
// literal PUT /order route takes precedence over PUT /{id}.
func registerPinnedRoutes(mux *http.ServeMux, sessionKey []byte, store pinnedStore) {
	guestList, adminList := http.Handler(pinnedUnavailable), http.Handler(pinnedUnavailable)
	create, update, remove, reorder := http.Handler(pinnedUnavailable), http.Handler(pinnedUnavailable), http.Handler(pinnedUnavailable), http.Handler(pinnedUnavailable)
	if store != nil {
		guestList = pinnedGuestListHandler(store)
		adminList = pinnedAdminListHandler(store)
		create = pinnedCreateHandler(store)
		update = pinnedUpdateHandler(store)
		remove = pinnedDeleteHandler(store)
		reorder = pinnedReorderHandler(store)
	}
	mux.Handle("GET /api/pinned", noStore(auth.RequireGuestSession(sessionKey, guestList)))
	mux.Handle("GET /api/admin/pinned", noStore(auth.RequireAdminSession(sessionKey, adminList)))
	mux.Handle("POST /api/admin/pinned", noStore(auth.RequireAdminSession(sessionKey, create)))
	mux.Handle("PUT /api/admin/pinned/order", noStore(auth.RequireAdminSession(sessionKey, reorder)))
	mux.Handle("PUT /api/admin/pinned/{id}", noStore(auth.RequireAdminSession(sessionKey, update)))
	mux.Handle("DELETE /api/admin/pinned/{id}", noStore(auth.RequireAdminSession(sessionKey, remove)))
}

var pinnedUnavailable = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	writeUploadError(w, http.StatusServiceUnavailable, "pinned_unavailable")
})

// pinnedImage is the picture of an entry, or null when it is not published.
type pinnedImage struct {
	ID        string  `json:"id"`
	ThumbURL  string  `json:"thumb_url"`
	DetailURL *string `json:"detail_url"`
}

func toPinnedImage(image *pinned.Image) *pinnedImage {
	if image == nil {
		return nil
	}
	return &pinnedImage{ID: image.ID, ThumbURL: image.ThumbURL, DetailURL: image.DetailURL}
}

// guestPinnedItem is what guests see; management fields stay private.
type guestPinnedItem struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Body  string       `json:"body"`
	Image *pinnedImage `json:"image"`
}

type guestPinnedPage struct {
	Items []guestPinnedItem `json:"items"`
}

// adminPinnedItem is the administrator view of an entry.
type adminPinnedItem struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	MediaID   *string      `json:"media_id"`
	Position  int          `json:"position"`
	CreatedAt int64        `json:"created_at"`
	UpdatedAt int64        `json:"updated_at"`
	Image     *pinnedImage `json:"image"`
}

type adminPinnedPage struct {
	Items []adminPinnedItem `json:"items"`
}

func toAdminPinnedItem(item pinned.Item) adminPinnedItem {
	return adminPinnedItem{
		ID: item.ID, Title: item.Title, Body: item.Body, MediaID: item.MediaID,
		Position: item.Position, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Image: toPinnedImage(item.Image),
	}
}

func toAdminPinnedPage(items []pinned.Item) adminPinnedPage {
	page := adminPinnedPage{Items: make([]adminPinnedItem, 0, len(items))}
	for _, item := range items {
		page.Items = append(page.Items, toAdminPinnedItem(item))
	}
	return page
}

func pinnedGuestListHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items, err := store.List(r.Context())
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
			return
		}
		page := guestPinnedPage{Items: make([]guestPinnedItem, 0, len(items))}
		for _, item := range items {
			page.Items = append(page.Items, guestPinnedItem{
				ID: item.ID, Title: item.Title, Body: item.Body, Image: toPinnedImage(item.Image),
			})
		}
		writeJSON(w, http.StatusOK, page)
	})
}

func pinnedAdminListHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items, err := store.List(r.Context())
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
			return
		}
		writeJSON(w, http.StatusOK, toAdminPinnedPage(items))
	})
}

func pinnedCreateHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, ok := readPinnedInput(w, r)
		if !ok {
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_item")
			return
		}
		item, err := store.Create(r.Context(), input)
		switch {
		case errors.Is(err, pinned.ErrInvalid):
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_item")
		case errors.Is(err, pinned.ErrLimitReached):
			writeUploadError(w, http.StatusConflict, "pinned_limit_reached")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
		default:
			writeJSON(w, http.StatusCreated, toAdminPinnedItem(item))
		}
	})
}

func pinnedUpdateHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, ok := readPinnedInput(w, r)
		if !ok {
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_item")
			return
		}
		item, err := store.Update(r.Context(), r.PathValue("id"), input)
		switch {
		case errors.Is(err, pinned.ErrInvalid):
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_item")
		case errors.Is(err, pinned.ErrNotFound):
			writeUploadError(w, http.StatusNotFound, "pinned_not_found")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
		default:
			writeJSON(w, http.StatusOK, toAdminPinnedItem(item))
		}
	})
}

func pinnedDeleteHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := store.Delete(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, pinned.ErrNotFound):
			writeUploadError(w, http.StatusNotFound, "pinned_not_found")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// pinnedOrderBody is {"ids":[...]}; a pointer distinguishes missing or null from [].
type pinnedOrderBody struct {
	IDs *[]string `json:"ids"`
}

func pinnedReorderHandler(store pinnedStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body pinnedOrderBody
		if err := decodeSingleJSON(w, r, maxPinnedBodyBytes, &body); err != nil || body.IDs == nil {
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_order")
			return
		}
		items, err := store.Reorder(r.Context(), *body.IDs)
		switch {
		case errors.Is(err, pinned.ErrInvalidOrder):
			writeUploadError(w, http.StatusBadRequest, "invalid_pinned_order")
		case err != nil:
			writeUploadError(w, http.StatusInternalServerError, "pinned_failed")
		default:
			writeJSON(w, http.StatusOK, toAdminPinnedPage(items))
		}
	})
}

// readPinnedInput decodes the entry body. title, body and media_id are all
// required; media_id may be null for an entry without image.
func readPinnedInput(w http.ResponseWriter, r *http.Request) (pinned.Input, bool) {
	var body struct {
		Title   *string         `json:"title"`
		Body    *string         `json:"body"`
		MediaID json.RawMessage `json:"media_id"`
	}
	if err := decodeSingleJSON(w, r, maxPinnedBodyBytes, &body); err != nil {
		return pinned.Input{}, false
	}
	if body.Title == nil || body.Body == nil || len(body.MediaID) == 0 {
		return pinned.Input{}, false
	}
	input := pinned.Input{Title: *body.Title, Body: *body.Body}
	if string(body.MediaID) != "null" {
		var mediaID string
		if err := json.Unmarshal(body.MediaID, &mediaID); err != nil {
			return pinned.Input{}, false
		}
		input.MediaID = &mediaID
	}
	return input, true
}

// decodeSingleJSON reads exactly one JSON object of at most limit bytes with
// no unknown fields and no trailing content.
func decodeSingleJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}
