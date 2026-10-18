package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// countingBody records whether the handler touched the request body at all.
type countingBody struct {
	reader io.Reader
	reads  int
}

func (b *countingBody) Read(p []byte) (int, error) {
	b.reads++
	return b.reader.Read(p)
}

func (b *countingBody) Close() error { return nil }

func TestPausedUploadsRejectWithoutReadingBodyOrWritingFiles(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	fx := newAdminFixture(t, store, nil)
	if err := fx.settings.SetUploadsEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}

	put := photoRequest(testMediaID, jpeg, guest)
	putBody := &countingBody{reader: bytes.NewReader(jpeg)}
	put.Body = putBody
	response := fx.do(put)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("paused PUT = %d %q; want 503 uploads_paused", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "" {
		t.Errorf("paused PUT sent Retry-After %q; want none so clients do not retry-storm", response.Header().Get("Retry-After"))
	}

	derived := derivativeRequest(testMediaID, "t", jpeg, guest)
	derivedBody := &countingBody{reader: bytes.NewReader(jpeg)}
	derived.Body = derivedBody
	response = fx.do(derived)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("paused derivative POST = %d %q; want 503 uploads_paused", response.Code, response.Body.String())
	}
	if putBody.reads != 0 || derivedBody.reads != 0 {
		t.Fatalf("paused uploads read the body (%d, %d times); want none", putBody.reads, derivedBody.reads)
	}
	for _, directory := range []string{paths.Tmp, paths.Originals, paths.Derived} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("paused upload left files in %s: %v, %v", directory, entries, err)
		}
	}
}

func TestPausedUploadIsRefusedBeforeCapacityCheck(t *testing.T) {
	photos := &blockingPhotoUploader{started: make(chan struct{}), release: make(chan struct{})}
	fx := newAdminFixture(t, photos, nil)
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		fx.do(photoRequest("123e4567-e89b-42d3-a456-426614174030", []byte("raw"), guest))
	}()
	select {
	case <-photos.started:
	case <-time.After(time.Second):
		close(photos.release)
		t.Fatal("first upload did not occupy the only slot")
	}
	defer func() {
		close(photos.release)
		<-firstDone
	}()

	if err := fx.settings.SetUploadsEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	response := fx.do(photoRequest("123e4567-e89b-42d3-a456-426614174031", []byte("raw"), guest))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("paused PUT while saturated = %d %q; want uploads_paused, not capacity", response.Code, response.Body.String())
	}
}

func TestResumedUploadsAcceptPhotosAgain(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	fx := newAdminFixture(t, store, nil)
	ctx := context.Background()
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}

	if err := fx.settings.SetUploadsEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if response := fx.do(photoRequest(testMediaID, body, guest)); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("paused PUT = %d; want 503", response.Code)
	}
	if err := fx.settings.SetUploadsEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if response := fx.do(photoRequest(testMediaID, body, guest)); response.Code != http.StatusCreated {
		t.Fatalf("resumed PUT = %d %q; want 201", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(paths.Originals)
	if err != nil || len(entries) != 1 {
		t.Fatalf("originals after resumed upload = %v, %v; want one file", entries, err)
	}
}

func TestGalleryHealthAndSessionStayAvailableWhilePaused(t *testing.T) {
	fx := newAdminFixture(t, nil, fakeGallery{})
	if err := fx.settings.SetUploadsEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	gallery := fx.do(adminRequest(http.MethodGet, "/api/media", "", guest))
	if gallery.Code != http.StatusOK || gallery.Body.String() != "{\"items\":[],\"next_cursor\":null}\n" {
		t.Errorf("gallery while paused = %d %q; want 200 empty page", gallery.Code, gallery.Body.String())
	}
	session := fx.do(adminRequest(http.MethodGet, "/api/session", "", guest))
	if session.Code != http.StatusNoContent {
		t.Errorf("session check while paused = %d; want 204", session.Code)
	}
	health := fx.do(adminRequest(http.MethodGet, "/api/health", ""))
	if health.Code != http.StatusOK {
		t.Errorf("health while paused = %d; want 200", health.Code)
	}
}
