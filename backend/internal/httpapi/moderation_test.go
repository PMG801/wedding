package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/upload"
)

const moderatedID = "123e4567-e89b-42d3-a456-426614174000"

type visibilityCall struct{ id, visibility string }

// fakeModerator records every SetVisibility call; rejected requests must not reach it.
type fakeModerator struct {
	calls []visibilityCall
	err   error
}

func (m *fakeModerator) SetVisibility(_ context.Context, id, visibility string) (upload.Media, error) {
	m.calls = append(m.calls, visibilityCall{id, visibility})
	if m.err != nil {
		return upload.Media{}, m.err
	}
	return upload.Media{ID: id, Visibility: visibility}, nil
}

func TestModerationPatchSetsVisibility(t *testing.T) {
	moderator := &fakeModerator{}
	fx := newAdminFixture(t, nil, nil, WithModeration(moderator))
	cookie := fx.adminCookie(t)

	for _, visibility := range []string{"hidden", "visible"} {
		response := fx.do(adminRequest(http.MethodPatch, "/api/admin/media/"+moderatedID, `{"visibility":"`+visibility+`"}`, cookie))
		want := `{"id":"` + moderatedID + `","visibility":"` + visibility + "\"}\n"
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("PATCH %s = %d %q; want 200 %q", visibility, response.Code, response.Body.String(), want)
		}
		if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q; want no-store", got)
		}
	}
	want := []visibilityCall{{moderatedID, "hidden"}, {moderatedID, "visible"}}
	if len(moderator.calls) != 2 || moderator.calls[0] != want[0] || moderator.calls[1] != want[1] {
		t.Fatalf("calls = %+v; want %+v", moderator.calls, want)
	}
}

func TestModerationPatchRejectsInvalidBodiesWithoutChanges(t *testing.T) {
	moderator := &fakeModerator{}
	fx := newAdminFixture(t, nil, nil, WithModeration(moderator))
	cookie := fx.adminCookie(t)

	for name, body := range map[string]string{
		"empty":          ``,
		"not json":       `hidden`,
		"missing field":  `{}`,
		"unknown field":  `{"visibility":"hidden","delete":true}`,
		"invalid value":  `{"visibility":"deleted"}`,
		"wrong type":     `{"visibility":true}`,
		"oversized body": `{"visibility":"hidden","x":"` + string(make([]byte, 2048)) + `"}`,
	} {
		response := fx.do(adminRequest(http.MethodPatch, "/api/admin/media/"+moderatedID, body, cookie))
		if response.Code != http.StatusBadRequest || response.Body.String() != "{\"error\":\"invalid_visibility\"}\n" {
			t.Fatalf("%s = %d %q; want 400 invalid_visibility", name, response.Code, response.Body.String())
		}
	}
	if len(moderator.calls) != 0 {
		t.Fatalf("invalid bodies reached the store: %+v", moderator.calls)
	}
}

func TestModerationPatchRequiresAdminSession(t *testing.T) {
	moderator := &fakeModerator{}
	fx := newAdminFixture(t, nil, nil, WithModeration(moderator))
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	for name, cookies := range map[string][]*http.Cookie{"no cookie": nil, "guest cookie": {guest}} {
		response := fx.do(adminRequest(http.MethodPatch, "/api/admin/media/"+moderatedID, `{"visibility":"hidden"}`, cookies...))
		if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"admin_session_required\"}\n" {
			t.Fatalf("%s = %d %q; want 401 admin_session_required", name, response.Code, response.Body.String())
		}
	}
	if len(moderator.calls) != 0 {
		t.Fatalf("unauthenticated requests reached the store: %+v", moderator.calls)
	}
}

func TestModerationPatchMapsStoreErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		body   string
	}{
		{upload.ErrMediaNotFound, http.StatusNotFound, "{\"error\":\"media_not_found\"}\n"},
		{upload.ErrInvalidVisibility, http.StatusBadRequest, "{\"error\":\"invalid_visibility\"}\n"},
		{errors.New("disk failure"), http.StatusInternalServerError, "{\"error\":\"moderation_failed\"}\n"},
	} {
		moderator := &fakeModerator{err: test.err}
		fx := newAdminFixture(t, nil, nil, WithModeration(moderator))
		response := fx.do(adminRequest(http.MethodPatch, "/api/admin/media/"+moderatedID, `{"visibility":"hidden"}`, fx.adminCookie(t)))
		if response.Code != test.status || response.Body.String() != test.body {
			t.Fatalf("%v = %d %q; want %d %q", test.err, response.Code, response.Body.String(), test.status, test.body)
		}
	}
}

func TestModerationPatchIsUnavailableWithoutModerator(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	response := fx.do(adminRequest(http.MethodPatch, "/api/admin/media/"+moderatedID, `{"visibility":"hidden"}`, fx.adminCookie(t)))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"moderation_unavailable\"}\n" {
		t.Fatalf("PATCH = %d %q; want 503 moderation_unavailable", response.Code, response.Body.String())
	}
}

func TestModerationHasNoDeleteRoute(t *testing.T) {
	moderator := &fakeModerator{}
	fx := newAdminFixture(t, nil, nil, WithModeration(moderator))
	response := fx.do(adminRequest(http.MethodDelete, "/api/admin/media/"+moderatedID, "", fx.adminCookie(t)))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE = %d; want 405", response.Code)
	}
	if len(moderator.calls) != 0 {
		t.Fatalf("DELETE reached the store: %+v", moderator.calls)
	}
}

type fakeHiddenMedia struct {
	page    media.Page
	err     error
	cursors []string
	limits  []int
	preview string
}

func (f *fakeHiddenMedia) ListHidden(_ context.Context, cursor string, limit int) (media.Page, error) {
	f.cursors, f.limits = append(f.cursors, cursor), append(f.limits, limit)
	return f.page, f.err
}

func (f *fakeHiddenMedia) OpenHiddenPreview(_ context.Context, id, variant string) (*os.File, error) {
	if f.preview == "" || id != moderatedID || variant != "t" {
		return nil, media.ErrPreviewNotFound
	}
	return os.Open(f.preview)
}

func TestHiddenListingReturnsAdminPage(t *testing.T) {
	thumb := "/api/admin/media/" + moderatedID + "/preview/t"
	hidden := &fakeHiddenMedia{page: media.Page{Items: []media.Item{{ID: moderatedID, MIME: "image/jpeg", ConfirmedAt: 7, ThumbURL: &thumb}}}}
	fx := newAdminFixture(t, nil, nil, WithHiddenMedia(hidden))
	cookie := fx.adminCookie(t)

	response := fx.do(adminRequest(http.MethodGet, "/api/admin/media?visibility=hidden&cursor=abc&limit=10", "", cookie))
	want := `{"items":[{"id":"` + moderatedID + `","mime":"image/jpeg","confirmed_at":7,"original_url":null,"thumb_url":"` + thumb + `","detail_url":null}],"next_cursor":null}` + "\n"
	if response.Code != http.StatusOK || response.Body.String() != want || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("listing = %d %q (%q); want 200 %q no-store", response.Code, response.Body.String(), response.Header().Get("Cache-Control"), want)
	}
	if len(hidden.cursors) != 1 || hidden.cursors[0] != "abc" || hidden.limits[0] != 10 {
		t.Fatalf("calls = %v %v; want cursor abc, limit 10", hidden.cursors, hidden.limits)
	}
	if response := fx.do(adminRequest(http.MethodGet, "/api/admin/media?visibility=hidden", "", cookie)); response.Code != http.StatusOK || hidden.limits[1] != 50 {
		t.Fatalf("default limit = %d, %v; want 200 and limit 50", response.Code, hidden.limits)
	}
}

func TestHiddenListingRejectsInvalidQueriesAndGuests(t *testing.T) {
	hidden := &fakeHiddenMedia{err: media.ErrInvalidCursor}
	fx := newAdminFixture(t, nil, nil, WithHiddenMedia(hidden))
	cookie := fx.adminCookie(t)
	for query, want := range map[string]string{
		"":                             "{\"error\":\"invalid_visibility\"}\n",
		"?visibility=visible":          "{\"error\":\"invalid_visibility\"}\n",
		"?visibility=hidden&limit=0":   "{\"error\":\"invalid_limit\"}\n",
		"?visibility=hidden&limit=101": "{\"error\":\"invalid_limit\"}\n",
		"?visibility=hidden&cursor=x":  "{\"error\":\"invalid_cursor\"}\n",
	} {
		response := fx.do(adminRequest(http.MethodGet, "/api/admin/media"+query, "", cookie))
		if response.Code != http.StatusBadRequest || response.Body.String() != want {
			t.Fatalf("%q = %d %q; want 400 %q", query, response.Code, response.Body.String(), want)
		}
	}
	if len(hidden.cursors) != 1 {
		t.Fatalf("invalid queries reached the store %d times; want only the cursor check", len(hidden.cursors))
	}
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	for _, path := range []string{"/api/admin/media?visibility=hidden", "/api/admin/media/" + moderatedID + "/preview/t"} {
		if response := fx.do(adminRequest(http.MethodGet, path, "", guest)); response.Code != http.StatusUnauthorized {
			t.Fatalf("guest %s = %d; want 401", path, response.Code)
		}
	}
}

func TestHiddenPreviewServesPrivateJPEGToAdminOnly(t *testing.T) {
	preview := filepath.Join(t.TempDir(), "t.staged")
	if err := os.WriteFile(preview, []byte("\xff\xd8\xffjpeg"), 0600); err != nil {
		t.Fatal(err)
	}
	fx := newAdminFixture(t, nil, nil, WithHiddenMedia(&fakeHiddenMedia{preview: preview}))
	cookie := fx.adminCookie(t)

	response := fx.do(adminRequest(http.MethodGet, "/api/admin/media/"+moderatedID+"/preview/t", "", cookie))
	if response.Code != http.StatusOK || response.Body.String() != "\xff\xd8\xffjpeg" {
		t.Fatalf("preview = %d %q; want 200 JPEG", response.Code, response.Body.String())
	}
	for header, want := range map[string]string{"Content-Type": "image/jpeg", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff"} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s = %q; want %q", header, got, want)
		}
	}
	missing := fx.do(adminRequest(http.MethodGet, "/api/admin/media/"+moderatedID+"/preview/d", "", cookie))
	if missing.Code != http.StatusNotFound || missing.Body.String() != "{\"error\":\"preview_not_found\"}\n" {
		t.Fatalf("missing preview = %d %q; want 404 preview_not_found", missing.Code, missing.Body.String())
	}
}

func TestHiddenRoutesAreUnavailableWithoutSource(t *testing.T) {
	fx := newAdminFixture(t, nil, nil)
	cookie := fx.adminCookie(t)
	for _, path := range []string{"/api/admin/media?visibility=hidden", "/api/admin/media/" + moderatedID + "/preview/t"} {
		response := fx.do(adminRequest(http.MethodGet, path, "", cookie))
		if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"moderation_unavailable\"}\n" {
			t.Fatalf("%s = %d %q; want 503 moderation_unavailable", path, response.Code, response.Body.String())
		}
	}
}
