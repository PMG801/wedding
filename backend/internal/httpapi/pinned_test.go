package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/pinned"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

const (
	pinVisibleBoth = "11111111-1111-4111-8111-111111111111"
	pinThumbOnly   = "22222222-2222-4222-8222-222222222222"
	pinHidden      = "33333333-3333-4333-8333-333333333333"
	pinNoThumb     = "44444444-4444-4444-8444-444444444444"
	pinUnknown     = "55555555-5555-4555-8555-555555555555"
)

type pinnedFixture struct {
	handler  http.Handler
	database *sql.DB
	paths    storage.Paths
	store    *pinned.Store
	key      []byte
}

func newPinnedFixture(t *testing.T, extra ...Option) *pinnedFixture {
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
	store, err := pinned.New(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	insertPinnedMedia(t, database, pinVisibleBoth, "visible", 1000)
	insertPinnedMedia(t, database, pinThumbOnly, "visible", 1001)
	insertPinnedMedia(t, database, pinHidden, "hidden", 1002)
	insertPinnedMedia(t, database, pinNoThumb, "visible", 1003)
	publishPinnedFile(t, paths, pinVisibleBoth, "t")
	publishPinnedFile(t, paths, pinVisibleBoth, "d")
	publishPinnedFile(t, paths, pinThumbOnly, "t")
	publishPinnedFile(t, paths, pinHidden, "t")
	publishPinnedFile(t, paths, pinHidden, "d")

	options := append([]Option{WithPinned(store)}, extra...)
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, gallery, options...)
	return &pinnedFixture{handler: handler, database: database, paths: paths, store: store, key: []byte(testSessionKey)}
}

func insertPinnedMedia(t *testing.T, database *sql.DB, id, visibility string, confirmedAt int64) {
	t.Helper()
	if _, err := database.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES (?, ?, 'image/jpeg', 3, ?, ?)",
		id, "name_"+id+".jpg", confirmedAt, visibility); err != nil {
		t.Fatal(err)
	}
}

func publishPinnedFile(t *testing.T, paths storage.Paths, id, variant string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(paths.Derived, upload.DerivativeFileName(id, variant)), []byte(variant), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *pinnedFixture) request(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *pinnedFixture) guest() *http.Cookie {
	return auth.NewGuestCookie(f.key, time.Now())
}

func (f *pinnedFixture) admin() *http.Cookie {
	return auth.NewAdminCookie(f.key, time.Now())
}

func (f *pinnedFixture) rowCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.database.QueryRow("SELECT count(*) FROM pinned_items").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// pinnedOrder returns the stored ids in position order.
func (f *pinnedFixture) pinnedOrder(t *testing.T) []string {
	t.Helper()
	items, err := f.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// imageJSON is the expected image object; detail is false when the detail
// derivative is not published.
func imageJSON(id string, detail bool) string {
	if !detail {
		return fmt.Sprintf(`{"id":%s,"thumb_url":"/m/d/%s_t.jpg","detail_url":null}`, jsonString(id), id)
	}
	return fmt.Sprintf(`{"id":%s,"thumb_url":"/m/d/%s_t.jpg","detail_url":"/m/d/%s_d.jpg"}`, jsonString(id), id, id)
}

func guestItemJSON(item pinned.Item, image string) string {
	return fmt.Sprintf(`{"id":%s,"title":%s,"body":%s,"image":%s}`, jsonString(item.ID), jsonString(item.Title), jsonString(item.Body), image)
}

// adminItemJSON renders the administrator shape; mediaID "" means null.
func adminItemJSON(item pinned.Item, mediaID, image string) string {
	media := "null"
	if mediaID != "" {
		media = jsonString(mediaID)
	}
	return fmt.Sprintf(`{"id":%s,"title":%s,"body":%s,"media_id":%s,"position":%d,"created_at":%d,"updated_at":%d,"image":%s}`,
		jsonString(item.ID), jsonString(item.Title), jsonString(item.Body), media, item.Position, item.CreatedAt, item.UpdatedAt, image)
}

func assertPinnedResponse(t *testing.T, response *httptest.ResponseRecorder, status int, wantBody string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d %q; want %d", response.Code, response.Body.String(), status)
	}
	if response.Body.String() != wantBody {
		t.Fatalf("body = %q; want %q", response.Body.String(), wantBody)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q; want no-store", response.Header().Get("Cache-Control"))
	}
}

func assertPinnedError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assertPinnedResponse(t, response, status, `{"error":"`+code+`"}`+"\n")
}

func TestPinnedRoutesRequireTheRightSession(t *testing.T) {
	fx := newPinnedFixture(t)
	item, err := fx.store.Create(context.Background(), pinned.Input{Title: "Aviso"})
	if err != nil {
		t.Fatal(err)
	}
	before := fx.pinnedOrder(t)
	guestOnly := []struct {
		method, path, body string
	}{{http.MethodGet, "/api/pinned", ""}}
	adminOnly := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/admin/pinned", ""},
		{http.MethodPost, "/api/admin/pinned", `{"title":"x","body":"","media_id":null}`},
		{http.MethodPut, "/api/admin/pinned/" + item.ID, `{"title":"x","body":"","media_id":null}`},
		{http.MethodDelete, "/api/admin/pinned/" + item.ID, ""},
		{http.MethodPut, "/api/admin/pinned/order", `{"ids":[]}`},
	}
	for _, route := range guestOnly {
		assertPinnedResponse(t, fx.request(route.method, route.path, route.body, nil), http.StatusUnauthorized, "guest session required\n")
		assertPinnedResponse(t, fx.request(route.method, route.path, route.body, fx.admin()), http.StatusUnauthorized, "guest session required\n")
	}
	for _, route := range adminOnly {
		assertPinnedError(t, fx.request(route.method, route.path, route.body, nil), http.StatusUnauthorized, "admin_session_required")
		assertPinnedError(t, fx.request(route.method, route.path, route.body, fx.guest()), http.StatusUnauthorized, "admin_session_required")
	}
	if got := fx.pinnedOrder(t); len(got) != 1 || got[0] != before[0] {
		t.Fatalf("unauthorized requests changed order: %v; want %v", got, before)
	}
}

func TestGuestListReturnsOrderedItemsWithResolvedImages(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	a, err := fx.store.Create(ctx, pinned.Input{Title: "Horario del día", Body: "13:00 ceremonia\n14:30 aperitivo", MediaID: ptrString(pinVisibleBoth)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := fx.store.Create(ctx, pinned.Input{Title: "Juego", Body: "", MediaID: ptrString(pinThumbOnly)})
	if err != nil {
		t.Fatal(err)
	}
	c, err := fx.store.Create(ctx, pinned.Input{Title: "Oculta", Body: "x", MediaID: ptrString(pinHidden)})
	if err != nil {
		t.Fatal(err)
	}
	d, err := fx.store.Create(ctx, pinned.Input{Title: "Solo texto", Body: "y"})
	if err != nil {
		t.Fatal(err)
	}

	want := fmt.Sprintf(`{"items":[%s,%s,%s,%s]}`+"\n",
		guestItemJSON(a, imageJSON(pinVisibleBoth, true)),
		guestItemJSON(b, imageJSON(pinThumbOnly, false)),
		guestItemJSON(c, "null"),
		guestItemJSON(d, "null"),
	)
	// The body line break is encoded as \n inside JSON; jsonString already does that.
	response := fx.request(http.MethodGet, "/api/pinned", "", fx.guest())
	assertPinnedResponse(t, response, http.StatusOK, want)
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
}

func TestGuestListWithoutEntriesReturnsEmptyArray(t *testing.T) {
	fx := newPinnedFixture(t)
	assertPinnedResponse(t, fx.request(http.MethodGet, "/api/pinned", "", fx.guest()), http.StatusOK, `{"items":[]}`+"\n")
}

func TestPinnedImageFollowsMediaVisibilityWithoutTouchingEntry(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	item, err := fx.store.Create(ctx, pinned.Input{Title: "Foto", MediaID: ptrString(pinVisibleBoth)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.database.Exec("UPDATE media SET visibility = 'hidden' WHERE id = ?", pinVisibleBoth); err != nil {
		t.Fatal(err)
	}
	assertPinnedResponse(t, fx.request(http.MethodGet, "/api/pinned", "", fx.guest()), http.StatusOK,
		fmt.Sprintf(`{"items":[%s]}`+"\n", guestItemJSON(item, "null")))
	if _, err := fx.database.Exec("UPDATE media SET visibility = 'visible' WHERE id = ?", pinVisibleBoth); err != nil {
		t.Fatal(err)
	}
	assertPinnedResponse(t, fx.request(http.MethodGet, "/api/pinned", "", fx.guest()), http.StatusOK,
		fmt.Sprintf(`{"items":[%s]}`+"\n", guestItemJSON(item, imageJSON(pinVisibleBoth, true))))
}

func TestPinnedItemsDoNotChangeGalleryListing(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	before := fx.request(http.MethodGet, "/api/media", "", fx.guest())
	if before.Code != http.StatusOK {
		t.Fatalf("gallery before = %d %q", before.Code, before.Body.String())
	}
	if _, err := fx.store.Create(ctx, pinned.Input{Title: "Fijada", MediaID: ptrString(pinVisibleBoth)}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.Create(ctx, pinned.Input{Title: "Sin imagen"}); err != nil {
		t.Fatal(err)
	}
	after := fx.request(http.MethodGet, "/api/media", "", fx.guest())
	if after.Code != http.StatusOK || after.Body.String() != before.Body.String() {
		t.Fatalf("gallery after pinned = %d %q; want unchanged %q", after.Code, after.Body.String(), before.Body.String())
	}
}

func TestAdminListReturnsManagementShape(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	a, err := fx.store.Create(ctx, pinned.Input{Title: "Uno", Body: "a\nb", MediaID: ptrString(pinThumbOnly)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := fx.store.Create(ctx, pinned.Input{Title: "Dos"})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"items":[%s,%s]}`+"\n",
		adminItemJSON(a, pinThumbOnly, imageJSON(pinThumbOnly, false)),
		adminItemJSON(b, "", "null"),
	)
	assertPinnedResponse(t, fx.request(http.MethodGet, "/api/admin/pinned", "", fx.admin()), http.StatusOK, want)
}

func TestAdminCreateAppendsAndReturnsCreated(t *testing.T) {
	fx := newPinnedFixture(t)
	response := fx.request(http.MethodPost, "/api/admin/pinned",
		`{"title":"  Horario  ","body":"13:00 ceremonia\n14:30 aperitivo","media_id":"`+pinVisibleBoth+`"}`, fx.admin())
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d %q; want 201", response.Code, response.Body.String())
	}
	items, err := fx.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("stored = %d; want 1", len(items))
	}
	created := items[0]
	want := adminItemJSON(created, pinVisibleBoth, imageJSON(pinVisibleBoth, true)) + "\n"
	assertPinnedResponse(t, response, http.StatusCreated, want)
	if created.Title != "Horario" || created.Position != 0 {
		t.Fatalf("stored item = %+v; want trimmed title at position 0", created)
	}
}

func TestAdminCreateRejectsInvalidBodiesWithoutWriting(t *testing.T) {
	fx := newPinnedFixture(t)
	padded := `{"title":"a","body":"","media_id":null}` + strings.Repeat(" ", 9000)
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "not json", body: "{"},
		{name: "empty body", body: ""},
		{name: "array", body: `[]`},
		{name: "two values", body: `{"title":"a","body":"","media_id":null}{}`},
		{name: "unknown field", body: `{"title":"a","body":"","media_id":null,"extra":1}`},
		{name: "missing title", body: `{"body":"","media_id":null}`},
		{name: "missing body", body: `{"title":"a","media_id":null}`},
		{name: "missing media_id", body: `{"title":"a","body":""}`},
		{name: "null title", body: `{"title":null,"body":"","media_id":null}`},
		{name: "empty title", body: `{"title":"   ","body":"","media_id":null}`},
		{name: "title too long", body: `{"title":"` + strings.Repeat("a", 121) + `","body":"","media_id":null}`},
		{name: "body too long", body: `{"title":"a","body":"` + strings.Repeat("ñ", 2001) + `","media_id":null}`},
		{name: "carriage return in body", body: `{"title":"a","body":"a\r\nb","media_id":null}`},
		{name: "control in title", body: `{"title":"a\tb","body":"","media_id":null}`},
		{name: "media_id not a string", body: `{"title":"a","body":"","media_id":5}`},
		{name: "unknown media", body: `{"title":"a","body":"","media_id":"` + pinUnknown + `"}`},
		{name: "over 8 KiB", body: padded},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertPinnedError(t, fx.request(http.MethodPost, "/api/admin/pinned", test.body, fx.admin()), http.StatusBadRequest, "invalid_pinned_item")
			if n := fx.rowCount(t); n != 0 {
				t.Fatalf("stored rows = %d after rejected body; want 0", n)
			}
		})
	}
}

func TestAdminCreateSixthIsConflict(t *testing.T) {
	fx := newPinnedFixture(t)
	for range pinned.MaxItems {
		if _, err := fx.store.Create(context.Background(), pinned.Input{Title: "Entrada"}); err != nil {
			t.Fatal(err)
		}
	}
	before := fx.pinnedOrder(t)
	assertPinnedError(t, fx.request(http.MethodPost, "/api/admin/pinned", `{"title":"Sexta","body":"","media_id":null}`, fx.admin()), http.StatusConflict, "pinned_limit_reached")
	if got := fx.pinnedOrder(t); len(got) != len(before) {
		t.Fatalf("rows after rejected sixth = %d; want %d", len(got), len(before))
	}
}

func TestAdminUpdateReplacesContentAndKeepsPosition(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	if _, err := fx.store.Create(ctx, pinned.Input{Title: "Uno"}); err != nil {
		t.Fatal(err)
	}
	target, err := fx.store.Create(ctx, pinned.Input{Title: "Dos"})
	if err != nil {
		t.Fatal(err)
	}
	response := fx.request(http.MethodPut, "/api/admin/pinned/"+target.ID,
		`{"title":"Dos editada","body":"nuevo","media_id":"`+pinThumbOnly+`"}`, fx.admin())
	items, err := fx.store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[1].ID != target.ID {
		t.Fatalf("order after update = %+v", items)
	}
	assertPinnedResponse(t, response, http.StatusOK, adminItemJSON(items[1], pinThumbOnly, imageJSON(pinThumbOnly, false))+"\n")
}

func TestAdminUpdateErrors(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	item, err := fx.store.Create(ctx, pinned.Input{Title: "Uno", Body: "antes"})
	if err != nil {
		t.Fatal(err)
	}
	assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/"+pinUnknown, `{"title":"x","body":"","media_id":null}`, fx.admin()), http.StatusNotFound, "pinned_not_found")
	assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/"+item.ID, `{"title":"","body":"","media_id":null}`, fx.admin()), http.StatusBadRequest, "invalid_pinned_item")
	assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/"+item.ID, `{"title":"x","body":"","media_id":"`+pinUnknown+`"}`, fx.admin()), http.StatusBadRequest, "invalid_pinned_item")
	items, err := fx.store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Title != "Uno" || items[0].Body != "antes" {
		t.Fatalf("rejected update changed entry: %+v", items[0])
	}
}

func TestAdminDeleteReturnsNoContentAndCompacts(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	first, err := fx.store.Create(ctx, pinned.Input{Title: "Uno"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.store.Create(ctx, pinned.Input{Title: "Dos"})
	if err != nil {
		t.Fatal(err)
	}
	response := fx.request(http.MethodDelete, "/api/admin/pinned/"+first.ID, "", fx.admin())
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("delete = %d %q headers=%v; want 204 empty no-store", response.Code, response.Body.String(), response.Header())
	}
	items, err := fx.store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != second.ID || items[0].Position != 0 {
		t.Fatalf("after delete = %+v; want second at position 0", items)
	}
	assertPinnedError(t, fx.request(http.MethodDelete, "/api/admin/pinned/"+first.ID, "", fx.admin()), http.StatusNotFound, "pinned_not_found")
	assertPinnedError(t, fx.request(http.MethodDelete, "/api/admin/pinned/not-an-id", "", fx.admin()), http.StatusNotFound, "pinned_not_found")
}

func TestAdminReorderUsesLiteralRouteAndExactSet(t *testing.T) {
	fx := newPinnedFixture(t)
	ctx := context.Background()
	a, err := fx.store.Create(ctx, pinned.Input{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := fx.store.Create(ctx, pinned.Input{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := fx.store.Create(ctx, pinned.Input{Title: "C"})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{}`,
		`{"ids":null}`,
		`{"ids":"` + a.ID + `"}`,
		`{"ids":[1]}`,
		`{"ids":[]}`,
		`{"ids":["` + a.ID + `","` + a.ID + `","` + b.ID + `"]}`,
		`{"ids":["` + a.ID + `","` + b.ID + `"]}`,
		`{"ids":["` + a.ID + `","` + b.ID + `","` + c.ID + `","` + pinUnknown + `"]}`,
		`{"ids":["` + a.ID + `","` + b.ID + `","` + pinUnknown + `"]}`,
		`{"ids":[],"extra":1}`,
		`{`,
	} {
		assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/order", body, fx.admin()), http.StatusBadRequest, "invalid_pinned_order")
	}
	if got := fx.pinnedOrder(t); strings.Join(got, ",") != strings.Join([]string{a.ID, b.ID, c.ID}, ",") {
		t.Fatalf("rejected orders changed order to %v", got)
	}

	response := fx.request(http.MethodPut, "/api/admin/pinned/order", `{"ids":["`+c.ID+`","`+a.ID+`","`+b.ID+`"]}`, fx.admin())
	items, err := fx.store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"items":[%s,%s,%s]}`+"\n",
		adminItemJSON(items[0], "", "null"), adminItemJSON(items[1], "", "null"), adminItemJSON(items[2], "", "null"))
	assertPinnedResponse(t, response, http.StatusOK, want)
	if items[0].ID != c.ID || items[1].ID != a.ID || items[2].ID != b.ID {
		t.Fatalf("order = %v; want C, A, B", []string{items[0].ID, items[1].ID, items[2].ID})
	}
}

func TestPinnedRoutesAreUnavailableWithoutStore(t *testing.T) {
	fx := newPinnedFixture(t)
	handler := NewHandler(testEventToken, []byte(testSessionKey), nil, 1, time.Second, 1024, nil)
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	assertPinnedError(t, call(http.MethodGet, "/api/pinned", "", fx.guest()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodGet, "/api/admin/pinned", "", fx.admin()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodPost, "/api/admin/pinned", `{"title":"x","body":"","media_id":null}`, fx.admin()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodPut, "/api/admin/pinned/"+pinUnknown, `{"title":"x","body":"","media_id":null}`, fx.admin()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodDelete, "/api/admin/pinned/"+pinUnknown, "", fx.admin()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodPut, "/api/admin/pinned/order", `{"ids":[]}`, fx.admin()), http.StatusServiceUnavailable, "pinned_unavailable")
	assertPinnedError(t, call(http.MethodGet, "/api/admin/pinned", "", nil), http.StatusUnauthorized, "admin_session_required")
}

// failingPinnedStore fails every operation so each handler's 500 mapping runs.
type failingPinnedStore struct{}

var errPinnedBoom = errors.New("boom")

func (failingPinnedStore) List(context.Context) ([]pinned.Item, error) { return nil, errPinnedBoom }
func (failingPinnedStore) Create(context.Context, pinned.Input) (pinned.Item, error) {
	return pinned.Item{}, errPinnedBoom
}
func (failingPinnedStore) Update(context.Context, string, pinned.Input) (pinned.Item, error) {
	return pinned.Item{}, errPinnedBoom
}
func (failingPinnedStore) Delete(context.Context, string) error { return errPinnedBoom }
func (failingPinnedStore) Reorder(context.Context, []string) ([]pinned.Item, error) {
	return nil, errPinnedBoom
}

func TestPinnedStorageFailuresReturnPinnedFailed(t *testing.T) {
	fx := newPinnedFixture(t, WithPinned(failingPinnedStore{}))
	assertPinnedError(t, fx.request(http.MethodGet, "/api/pinned", "", fx.guest()), http.StatusInternalServerError, "pinned_failed")
	assertPinnedError(t, fx.request(http.MethodGet, "/api/admin/pinned", "", fx.admin()), http.StatusInternalServerError, "pinned_failed")
	assertPinnedError(t, fx.request(http.MethodPost, "/api/admin/pinned", `{"title":"x","body":"","media_id":null}`, fx.admin()), http.StatusInternalServerError, "pinned_failed")
	assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/"+pinUnknown, `{"title":"x","body":"","media_id":null}`, fx.admin()), http.StatusInternalServerError, "pinned_failed")
	assertPinnedError(t, fx.request(http.MethodDelete, "/api/admin/pinned/"+pinUnknown, "", fx.admin()), http.StatusInternalServerError, "pinned_failed")
	assertPinnedError(t, fx.request(http.MethodPut, "/api/admin/pinned/order", `{"ids":[]}`, fx.admin()), http.StatusInternalServerError, "pinned_failed")
}

func ptrString(s string) *string { return &s }

func TestAdminCreateAcceptsNullMediaForEntryWithoutImage(t *testing.T) {
	fx := newPinnedFixture(t)
	response := fx.request(http.MethodPost, "/api/admin/pinned", `{"title":"Aviso","body":"","media_id":null}`, fx.admin())
	items, err := fx.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("stored = %d; want 1", len(items))
	}
	assertPinnedResponse(t, response, http.StatusCreated, adminItemJSON(items[0], "", "null")+"\n")
}
