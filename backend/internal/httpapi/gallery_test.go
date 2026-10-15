package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

func TestGalleryRequiresGuestSessionAndListsConfirmedPhotos(t *testing.T) {
	store, gallery := newHTTPGallery(t)
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, gallery)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, galleryRequest("", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated gallery status = %d; want 401", unauthorized.Code)
	}

	empty := httptest.NewRecorder()
	handler.ServeHTTP(empty, galleryRequest("", cookie))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"items":[],"next_cursor":null}` {
		t.Fatalf("empty gallery response = %d %q; want 200 with empty items", empty.Code, empty.Body.String())
	}
	if empty.Header().Get("Cache-Control") != "no-store" || empty.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("gallery headers = %v; want no-store JSON", empty.Header())
	}

	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, photoRequest(testMediaID, []byte{0xff, 0xd8, 0xff, 1}, cookie))
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("photo upload status = %d; want 201", uploaded.Code)
	}

	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, galleryRequest("?limit=1", cookie))
	var page struct {
		Items []struct {
			ID          string  `json:"id"`
			MIME        string  `json:"mime"`
			ConfirmedAt int64   `json:"confirmed_at"`
			OriginalURL string  `json:"original_url"`
			ThumbURL    *string `json:"thumb_url"`
			DetailURL   *string `json:"detail_url"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || listed.Code != http.StatusOK {
		t.Fatalf("gallery response = %d %q (%v); want 200 JSON", listed.Code, listed.Body.String(), err)
	}
	if len(page.Items) != 1 || page.NextCursor != nil {
		t.Fatalf("gallery page = %+v; want one item and no next cursor", page)
	}
	item := page.Items[0]
	if item.ID != testMediaID || item.MIME != "image/jpeg" || item.ConfirmedAt <= 0 || !strings.HasPrefix(item.OriginalURL, "/m/o/") || !strings.HasSuffix(item.OriginalURL, "_"+testMediaID+".jpg") {
		t.Fatalf("gallery item = %+v; want confirmed JPEG with its public original URL", item)
	}
	if item.ThumbURL != nil || item.DetailURL != nil {
		t.Fatalf("gallery derivative URLs = %v %v; want null without derivatives", item.ThumbURL, item.DetailURL)
	}
}

func TestGalleryRejectsInvalidLimitAndCursor(t *testing.T) {
	store, gallery := newHTTPGallery(t)
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, gallery)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	for _, test := range []struct{ query, code string }{
		{"?limit=0", "invalid_limit"},
		{"?limit=101", "invalid_limit"},
		{"?limit=many", "invalid_limit"},
		{"?cursor=%21%21", "invalid_cursor"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, galleryRequest(test.query, cookie))
		if response.Code != http.StatusBadRequest || strings.TrimSpace(response.Body.String()) != `{"error":"`+test.code+`"}` {
			t.Fatalf("gallery %s response = %d %q; want 400 %s", test.query, response.Code, response.Body.String(), test.code)
		}
	}
}

func TestGalleryUnavailableWithoutLister(t *testing.T) {
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, galleryRequest("", auth.NewGuestCookie([]byte(testSessionKey), time.Now())))
	if response.Code != http.StatusServiceUnavailable || !bytes.Contains(response.Body.Bytes(), []byte("gallery_unavailable")) {
		t.Fatalf("gallery without lister = %d %q; want 503 gallery_unavailable", response.Code, response.Body.String())
	}
}

func galleryRequest(query string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/media"+query, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func newHTTPGallery(t *testing.T) (*upload.Store, *media.Gallery) {
	t.Helper()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := upload.NewStore(database, paths, upload.Options{MaxBytes: 1024, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	gallery, err := media.NewGallery(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	return store, gallery
}
