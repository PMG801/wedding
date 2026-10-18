package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
)

func topRequest(query string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/media/top"+query, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func TestTopRequiresGuestSessionAndReturnsLikedItemsEnvelope(t *testing.T) {
	fx := newLikeFixture(t)
	guest := fx.guest(t)
	handler := fx.handler

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, topRequest("", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Body.String() != "guest session required\n" {
		t.Fatalf("top without session = %d %q; want 401 text", unauthorized.Code, unauthorized.Body.String())
	}

	empty := httptest.NewRecorder()
	handler.ServeHTTP(empty, topRequest("", guest))
	if empty.Code != http.StatusOK || empty.Body.String() != "{\"items\":[]}\n" {
		t.Fatalf("empty top = %d %q; want 200 empty items", empty.Code, empty.Body.String())
	}
	if empty.Header().Get("Content-Type") != "application/json; charset=utf-8" || empty.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("top headers = %v; want JSON with no-store", empty.Header())
	}

	fx.do(http.MethodPut, likeVisibleID, guest)
	liked := httptest.NewRecorder()
	handler.ServeHTTP(liked, topRequest("?limit=1&cursor=ignored", guest))
	want := `{"items":[{"id":"` + likeVisibleID + `","mime":"image/jpeg","confirmed_at":1000,` +
		`"original_url":"/m/o/name_` + likeVisibleID + `.jpg","thumb_url":null,"detail_url":null,` +
		`"like_count":1,"liked_by_me":true}]}` + "\n"
	if liked.Code != http.StatusOK || liked.Body.String() != want {
		t.Fatalf("top after like = %d %q; want %q", liked.Code, liked.Body.String(), want)
	}
}

func TestTopIsUnavailableWithoutGallery(t *testing.T) {
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, topRequest("", auth.NewGuestCookie([]byte(testSessionKey), time.Now())))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"gallery_unavailable\"}\n" {
		t.Fatalf("top without gallery = %d %q; want 503 gallery_unavailable", response.Code, response.Body.String())
	}
}

func TestGalleryItemsCarryLikeFieldsForTheRequestingGuest(t *testing.T) {
	fx := newLikeFixture(t)
	mine, other := fx.guest(t), fx.guest(t)
	fx.do(http.MethodPut, likeVisibleID, mine)
	fx.do(http.MethodPut, likeVisibleID, other)

	response := httptest.NewRecorder()
	fx.handler.ServeHTTP(response, galleryRequest("", mine))
	var page struct {
		Items []struct {
			ID        string `json:"id"`
			LikeCount int64  `json:"like_count"`
			LikedByMe bool   `json:"liked_by_me"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || response.Code != http.StatusOK {
		t.Fatalf("gallery = %d %q (%v); want 200 JSON", response.Code, response.Body.String(), err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != likeVisibleID || page.Items[0].LikeCount != 2 || !page.Items[0].LikedByMe {
		t.Fatalf("gallery items = %+v; want the visible medium with count 2 liked by me", page.Items)
	}
}
