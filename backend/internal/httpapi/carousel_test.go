package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
)

// carouselNow is 18:05 UTC on the event day: inside the 18:00-20:00 period, in slot 18:00-18:30.
var carouselNow = time.Date(2026, 10, 18, 18, 5, 0, 0, time.UTC)

func newCarouselHandler(t *testing.T, periods []media.Period) http.Handler {
	t.Helper()
	store, gallery := newHTTPGallery(t)
	carousel, err := media.NewCarousel(gallery, media.CarouselConfig{
		Periods:  periods,
		Size:     10,
		Rotation: 30 * time.Minute,
		Now:      func() time.Time { return carouselNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	resumable, err := store.Resumable(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, gallery, WithCarousel(carousel), WithResumable(resumable))
}

func carouselRequest(path string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func TestCarouselStripRequiresGuestSessionAndIsNotCached(t *testing.T) {
	handler := newCarouselHandler(t, []media.Period{{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T20:00"}})
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, carouselRequest("/api/carousels", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated strip = %d; want 401", unauthorized.Code)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, carouselRequest("/api/carousels", cookie))
	if response.Code != http.StatusOK {
		t.Fatalf("strip = %d %q; want 200", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("strip headers = %v; want no-store JSON", response.Header())
	}
	var body struct {
		RotatesAt int64 `json:"rotates_at"`
		Periods   []any `json:"periods"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	wantRotates := time.Date(2026, 10, 18, 18, 30, 0, 0, time.UTC).UnixMilli()
	if body.RotatesAt != wantRotates || body.Periods == nil || len(body.Periods) != 0 {
		t.Fatalf("strip body = %s; want rotates_at %d and an empty periods list", response.Body.String(), wantRotates)
	}
}

func TestCarouselStripAndAlbumShapeAndGalleryItems(t *testing.T) {
	handler := newCarouselHandler(t, []media.Period{{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T20:00"}})
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := []byte{0xff, 0xd8, 0xff, 1}
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, tusCreate(capturedMetadata(testMediaID, "2026-10-18T18:05:09", true), int64(len(body)), cookie))
	if created.Code != http.StatusCreated {
		t.Fatalf("create upload = %d %q; want 201", created.Code, created.Body.String())
	}
	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, tusPatch(testMediaID, 0, bytes.NewReader(body), int64(len(body)), cookie))
	if uploaded.Code != http.StatusNoContent {
		t.Fatalf("upload = %d %q; want 204", uploaded.Code, uploaded.Body.String())
	}

	strip := httptest.NewRecorder()
	handler.ServeHTTP(strip, carouselRequest("/api/carousels", cookie))
	var stripBody struct {
		Periods []struct {
			ID    int    `json:"id"`
			Start string `json:"start"`
			End   string `json:"end"`
			Total int64  `json:"total"`
			Items []struct {
				ID          string  `json:"id"`
				MIME        string  `json:"mime"`
				OriginalURL *string `json:"original_url"`
				LikeCount   int64   `json:"like_count"`
				LikedByMe   bool    `json:"liked_by_me"`
			} `json:"items"`
		} `json:"periods"`
	}
	if err := json.Unmarshal(strip.Body.Bytes(), &stripBody); err != nil || strip.Code != http.StatusOK {
		t.Fatalf("strip = %d %q (%v); want 200 JSON", strip.Code, strip.Body.String(), err)
	}
	if len(stripBody.Periods) != 1 || stripBody.Periods[0].ID != 1 || stripBody.Periods[0].Start != "2026-10-18T18:00" || stripBody.Periods[0].Total != 1 {
		t.Fatalf("strip periods = %+v; want period 1 with one visible medium", stripBody.Periods)
	}
	item := stripBody.Periods[0].Items[0]
	if item.ID != testMediaID || item.MIME != "image/jpeg" || item.OriginalURL == nil || !strings.HasPrefix(*item.OriginalURL, "/m/o/") {
		t.Fatalf("strip item = %+v; want the gallery item shape", item)
	}

	album := httptest.NewRecorder()
	handler.ServeHTTP(album, carouselRequest("/api/carousels/1?limit=1", cookie))
	if album.Code != http.StatusOK || album.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("album = %d %v %q; want 200 no-store", album.Code, album.Header(), album.Body.String())
	}
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(album.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != nil || page.Items[0]["id"] != testMediaID {
		t.Fatalf("album page = %s; want one item and no next cursor", album.Body.String())
	}
}

func TestCarouselAlbumRejectsUnknownPeriodAndBadParameters(t *testing.T) {
	handler := newCarouselHandler(t, []media.Period{{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T20:00"}})
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, carouselRequest("/api/carousels/1", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated album = %d; want 401", unauthorized.Code)
	}
	for _, test := range []struct {
		path, code string
		status     int
	}{
		{"/api/carousels/2", "period_not_found", http.StatusNotFound},
		{"/api/carousels/0", "period_not_found", http.StatusNotFound},
		{"/api/carousels/abc", "period_not_found", http.StatusNotFound},
		{"/api/carousels/1?limit=0", "invalid_limit", http.StatusBadRequest},
		{"/api/carousels/1?limit=101", "invalid_limit", http.StatusBadRequest},
		{"/api/carousels/1?limit=many", "invalid_limit", http.StatusBadRequest},
		{"/api/carousels/1?cursor=%21%21", "invalid_cursor", http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, carouselRequest(test.path, cookie))
		if response.Code != test.status || strings.TrimSpace(response.Body.String()) != `{"error":"`+test.code+`"}` {
			t.Fatalf("%s = %d %q; want %d %s", test.path, response.Code, response.Body.String(), test.status, test.code)
		}
	}
}

func TestCarouselUnavailableWithoutReader(t *testing.T) {
	store, gallery := newHTTPGallery(t)
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, gallery)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	for _, path := range []string{"/api/carousels", "/api/carousels/1"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, carouselRequest(path, cookie))
		if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"error":"carousels_unavailable"}` {
			t.Fatalf("%s without reader = %d %q; want 503 carousels_unavailable", path, response.Code, response.Body.String())
		}
	}
}
