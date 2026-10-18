package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/settings"
	"github.com/PMG801/wedding/internal/storage"
)

const (
	likeVisibleID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	likeHiddenID  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	likeUnknownID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

type likeFixture struct {
	handler  http.Handler
	database *sql.DB
	key      []byte
}

func newLikeFixture(t *testing.T, extra ...Option) *likeFixture {
	t.Helper()
	ctx := context.Background()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(ctx, filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	gallery, err := media.NewGallery(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	likes, err := media.NewLikes(database)
	if err != nil {
		t.Fatal(err)
	}
	insertLikeTestMedia(t, database, likeVisibleID, "visible", 1000)
	insertLikeTestMedia(t, database, likeHiddenID, "hidden", 2000)
	options := append([]Option{WithLikes(likes)}, extra...)
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, gallery, options...)
	return &likeFixture{handler: handler, database: database, key: []byte(testSessionKey)}
}

func insertLikeTestMedia(t *testing.T, database *sql.DB, id, visibility string, confirmedAt int64) {
	t.Helper()
	if _, err := database.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES (?, ?, 'image/jpeg', 3, ?, ?)",
		id, "name_"+id+".jpg", confirmedAt, visibility); err != nil {
		t.Fatal(err)
	}
}

func (f *likeFixture) do(method, id string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/media/"+id+"/like", nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

// counts returns the aggregate and the number of like rows for one medium.
func (f *likeFixture) counts(t *testing.T, id string) (int, int) {
	t.Helper()
	var count, rows int
	if err := f.database.QueryRow("SELECT like_count FROM media WHERE id = ?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := f.database.QueryRow("SELECT count(*) FROM media_likes WHERE media_id = ?", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return count, rows
}

func (f *likeFixture) guest(t *testing.T) *http.Cookie {
	t.Helper()
	return auth.NewGuestCookie(f.key, time.Now())
}

func assertLikeJSON(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %q; want 200", response.Code, response.Body.String())
	}
	if response.Body.String() != want {
		t.Fatalf("body = %q; want %q", response.Body.String(), want)
	}
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v; want JSON with no-store", response.Header())
	}
}

func TestPutLikeReturnsFinalStateAndIsIdempotent(t *testing.T) {
	fx := newLikeFixture(t)
	guest := fx.guest(t)

	assertLikeJSON(t, fx.do(http.MethodPut, likeVisibleID, guest),
		`{"id":"`+likeVisibleID+`","like_count":1,"liked_by_me":true}`+"\n")
	assertLikeJSON(t, fx.do(http.MethodPut, likeVisibleID, guest),
		`{"id":"`+likeVisibleID+`","like_count":1,"liked_by_me":true}`+"\n")
	if count, rows := fx.counts(t, likeVisibleID); count != 1 || rows != 1 {
		t.Fatalf("after repeated PUT count = %d rows = %d; want 1 and 1", count, rows)
	}
}

func TestDeleteLikeReturnsFinalStateAndIsIdempotent(t *testing.T) {
	fx := newLikeFixture(t)
	guest := fx.guest(t)
	fx.do(http.MethodPut, likeVisibleID, guest)

	assertLikeJSON(t, fx.do(http.MethodDelete, likeVisibleID, guest),
		`{"id":"`+likeVisibleID+`","like_count":0,"liked_by_me":false}`+"\n")
	assertLikeJSON(t, fx.do(http.MethodDelete, likeVisibleID, guest),
		`{"id":"`+likeVisibleID+`","like_count":0,"liked_by_me":false}`+"\n")
	if count, rows := fx.counts(t, likeVisibleID); count != 0 || rows != 0 {
		t.Fatalf("after repeated DELETE count = %d rows = %d; want 0 and 0", count, rows)
	}
}

func TestDistinctGuestCookiesCountAsDistinctVoters(t *testing.T) {
	fx := newLikeFixture(t)
	first, second := fx.guest(t), fx.guest(t)

	fx.do(http.MethodPut, likeVisibleID, first)
	assertLikeJSON(t, fx.do(http.MethodPut, likeVisibleID, second),
		`{"id":"`+likeVisibleID+`","like_count":2,"liked_by_me":true}`+"\n")
	assertLikeJSON(t, fx.do(http.MethodDelete, likeVisibleID, first),
		`{"id":"`+likeVisibleID+`","like_count":1,"liked_by_me":false}`+"\n")
}

func TestQRReentryInSameBrowserKeepsVoterIdentity(t *testing.T) {
	fx := newLikeFixture(t)
	original := fx.guest(t)
	fx.do(http.MethodPut, likeVisibleID, original)

	entry := httptest.NewRequest(http.MethodGet, "/e/"+testEventToken, nil)
	entry.AddCookie(original)
	entered := httptest.NewRecorder()
	fx.handler.ServeHTTP(entered, entry)
	cookies := entered.Result().Cookies()
	if entered.Code != http.StatusSeeOther || len(cookies) != 1 {
		t.Fatalf("QR re-entry = %d cookies %v; want 303 with one cookie", entered.Code, cookies)
	}

	assertLikeJSON(t, fx.do(http.MethodPut, likeVisibleID, cookies[0]),
		`{"id":"`+likeVisibleID+`","like_count":1,"liked_by_me":true}`+"\n")
	if count, rows := fx.counts(t, likeVisibleID); count != 1 || rows != 1 {
		t.Fatalf("re-entry double-counted: count = %d rows = %d; want 1 and 1", count, rows)
	}
}

func TestLegacyCookieLikeUsesUpgradedIdentity(t *testing.T) {
	fx := newLikeFixture(t)
	legacy := legacyGuestCookieForTest(fx.key, time.Now().Add(48*time.Hour).UTC().Truncate(time.Second))

	upgraded := fx.do(http.MethodPut, likeVisibleID, legacy)
	cookies := upgraded.Result().Cookies()
	if upgraded.Code != http.StatusOK || len(cookies) != 1 || cookies[0].Name != auth.GuestCookieName {
		t.Fatalf("legacy PUT = %d cookies %v; want 200 with upgraded cookie", upgraded.Code, cookies)
	}
	assertLikeJSON(t, fx.do(http.MethodPut, likeVisibleID, cookies[0]),
		`{"id":"`+likeVisibleID+`","like_count":1,"liked_by_me":true}`+"\n")
}

func TestLikeRejectsMissingSessionAndInvalidIDWithoutWriting(t *testing.T) {
	fx := newLikeFixture(t)
	missing := fx.do(http.MethodPut, likeVisibleID, nil)
	if missing.Code != http.StatusUnauthorized || missing.Body.String() != "guest session required\n" {
		t.Fatalf("missing session = %d %q; want 401 text", missing.Code, missing.Body.String())
	}
	tampered := fx.guest(t)
	tampered.Value = "forged"
	if response := fx.do(http.MethodPut, likeVisibleID, tampered); response.Code != http.StatusUnauthorized {
		t.Fatalf("tampered session = %d; want 401", response.Code)
	}

	guest := fx.guest(t)
	for _, id := range []string{"not-a-uuid", strings.ToUpper(likeVisibleID), "aaaaaaaa-aaaa-1aaa-8aaa-aaaaaaaaaaaa"} {
		response := fx.do(http.MethodPut, id, guest)
		if response.Code != http.StatusBadRequest || response.Body.String() != "{\"error\":\"invalid_media_id\"}\n" {
			t.Fatalf("PUT id %q = %d %q; want 400 invalid_media_id", id, response.Code, response.Body.String())
		}
	}
	if count, rows := fx.counts(t, likeVisibleID); count != 0 || rows != 0 {
		t.Fatalf("rejected requests changed likes: count = %d rows = %d", count, rows)
	}
}

func TestLikeHiddenOrUnknownMediaIsNotFoundAndUnchanged(t *testing.T) {
	fx := newLikeFixture(t)
	guest := fx.guest(t)
	for _, id := range []string{likeHiddenID, likeUnknownID} {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			response := fx.do(method, id, guest)
			if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"media_not_found\"}\n" {
				t.Fatalf("%s %s = %d %q; want 404 media_not_found", method, id, response.Code, response.Body.String())
			}
		}
	}
	if count, rows := fx.counts(t, likeHiddenID); count != 0 || rows != 0 {
		t.Fatalf("hidden medium changed: count = %d rows = %d", count, rows)
	}
}

func TestLikeStorageFailureReturnsLikeFailedAndWritesNothing(t *testing.T) {
	fx := newLikeFixture(t)
	guest := fx.guest(t)
	fx.do(http.MethodPut, likeVisibleID, guest)
	if err := fx.database.Close(); err != nil {
		t.Fatal(err)
	}

	response := fx.do(http.MethodPut, likeVisibleID, guest)
	if response.Code != http.StatusInternalServerError || response.Body.String() != "{\"error\":\"like_failed\"}\n" {
		t.Fatalf("storage failure = %d %q; want 500 like_failed", response.Code, response.Body.String())
	}
}

func TestLikesUnavailableWithoutStore(t *testing.T) {
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, nil)
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	request := httptest.NewRequest(http.MethodPut, "/api/media/"+likeVisibleID+"/like", nil)
	request.AddCookie(guest)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"likes_unavailable\"}\n" {
		t.Fatalf("likes without store = %d %q; want 503 likes_unavailable", response.Code, response.Body.String())
	}
}

func TestLikesAreNotGatedByUploadsPause(t *testing.T) {
	ctx := context.Background()
	fx := newLikeFixture(t)
	switches, err := settings.Open(ctx, fx.database)
	if err != nil {
		t.Fatal(err)
	}
	if err := switches.SetUploadsEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	likes, err := media.NewLikes(fx.database)
	if err != nil {
		t.Fatal(err)
	}
	gallery, err := media.NewGallery(fx.database, storage.Paths{Derived: t.TempDir(), Tmp: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	paused := NewHandler(testEventToken, fx.key, nil, 1, time.Second, 1024, gallery,
		WithAdmin("", switches, nil), WithLikes(likes))

	request := httptest.NewRequest(http.MethodPut, "/api/media/"+likeVisibleID+"/like", nil)
	request.AddCookie(fx.guest(t))
	response := httptest.NewRecorder()
	paused.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT like while uploads paused = %d %q; want 200", response.Code, response.Body.String())
	}
}
