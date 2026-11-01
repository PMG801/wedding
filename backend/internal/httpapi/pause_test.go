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

	derived := derivativeRequest(testMediaID, "t", jpeg, guest)
	derivedBody := &countingBody{reader: bytes.NewReader(jpeg)}
	derived.Body = derivedBody
	response := fx.do(derived)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("paused derivative POST = %d %q; want 503 uploads_paused", response.Code, response.Body.String())
	}
	if derivedBody.reads != 0 {
		t.Fatalf("paused derivative read the body %d times; want none", derivedBody.reads)
	}
	for _, directory := range []string{paths.Tmp, paths.Originals, paths.Derived} {
		entries, err := readDirWithoutTus(t, directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("paused upload left files in %s: %v, %v", directory, entries, err)
		}
	}
}

// A paused switch answers before the shared capacity is checked, even while a
// derivative holds the only slot.
func TestPausedUploadIsRefusedBeforeCapacityCheck(t *testing.T) {
	photos := &blockingPhotoDerivativeUploader{started: make(chan struct{}), release: make(chan struct{})}
	fx := newAdminFixture(t, photos, nil)
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	jpeg := []byte{0xff, 0xd8, 0xff, 1}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		fx.do(derivativeRequest("123e4567-e89b-42d3-a456-426614174030", "t", jpeg, guest))
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
	response := fx.do(derivativeRequest("123e4567-e89b-42d3-a456-426614174031", "t", jpeg, guest))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("paused derivative while saturated = %d %q; want uploads_paused, not capacity", response.Code, response.Body.String())
	}
}

func TestResumedUploadsAcceptPhotosAgain(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	resumable, err := store.Resumable(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fx := newAdminFixture(t, store, nil, WithResumable(resumable))
	ctx := context.Background()
	guest := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}

	if err := fx.settings.SetUploadsEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if response := fx.do(tusCreate(idMetadata(testMediaID), int64(len(body)), guest)); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("paused tus creation = %d; want 503", response.Code)
	}
	if err := fx.settings.SetUploadsEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	seedOriginalViaTus(t, fx.handler, testMediaID, body, guest)
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
