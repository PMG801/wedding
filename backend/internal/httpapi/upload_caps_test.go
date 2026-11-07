package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

const (
	tooManyPending = "{\"error\":\"too_many_pending_uploads\"}\n"
	tooManyStaged  = "{\"error\":\"too_many_staged_derivatives\"}\n"
)

func TestResumableCreateAtPendingCapAnswers503WithRetryAfterAndWritesNothing(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MaxPendingUploads: 1}, 2)
	f.createUpload(tusIDOne, 100)
	before := f.tusFiles()

	response := f.serve(tusCreate(idMetadata(tusIDTwo), 100, f.cookie))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != tooManyPending {
		t.Fatalf("POST at the pending cap = %d %q; want 503 too_many_pending_uploads", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q; want 30", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q; want JSON", got)
	}
	if after := f.tusFiles(); !reflect.DeepEqual(before, after) {
		t.Fatalf("tus files after rejected create = %v; want %v", after, before)
	}
}

func TestResumableDuplicateCreateStillAnswers409BeforeThePendingCap(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MaxPendingUploads: 1}, 2)
	f.createUpload(tusIDOne, 100)
	response := f.serve(tusCreate(idMetadata(tusIDOne), 100, f.cookie))
	if response.Code != http.StatusConflict || response.Body.String() != "{\"error\":\"upload_in_progress\"}\n" {
		t.Fatalf("duplicate POST = %d %q; want 409 upload_in_progress", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "" {
		t.Fatalf("duplicate POST carries Retry-After %q; want none", response.Header().Get("Retry-After"))
	}
}

func TestResumableInsufficientStorageStillAnswers507UnderTheCap(t *testing.T) {
	options := upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MaxPendingUploads: 5, MinFreeBytes: 100, FreeSpace: func(string) (uint64, error) { return 150, nil }}
	f := newResumableFixture(t, options, 2)
	response := f.serve(tusCreate(idMetadata(tusIDOne), 100, f.cookie))
	if response.Code != http.StatusInsufficientStorage || response.Body.String() != "{\"error\":\"insufficient_storage\"}\n" {
		t.Fatalf("POST over free space = %d %q; want 507 insufficient_storage", response.Code, response.Body.String())
	}
	f.requireNoChanges("rejected create")
}

func ageUpload(t *testing.T, f *resumableFixture, id string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	for _, name := range []string{id, id + ".info"} {
		if err := os.Chtimes(filepath.Join(f.paths.Tus, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReconcileExpiresEmptyUploadsSoonerThanUploadsWithBytes(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 4, UploadResumeTTL: 24 * time.Hour, EmptyUploadTTL: 15 * time.Minute}, 4)
	f.createUpload(tusIDOne, 100) // empty, past the empty TTL
	f.createUpload(tusIDTwo, 100) // has bytes, past the empty TTL only
	if response := f.serve(tusPatch(tusIDTwo, 0, bytes.NewReader(jpegBody(10)), 10, f.cookie)); response.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %q; want 204", response.Code, response.Body.String())
	}
	ageUpload(t, f, tusIDOne, 30*time.Minute)
	ageUpload(t, f, tusIDTwo, 30*time.Minute)

	report, err := f.store.Reconcile(t.Context())
	if err != nil || report.ExpiredResumable != 1 {
		t.Fatalf("Reconcile() = %+v, %v; want one expired upload", report, err)
	}
	if got := f.tusFiles(); !reflect.DeepEqual(got, []string{tusIDTwo, tusIDTwo + ".info"}) {
		t.Fatalf("tus files = %v; want only the upload with bytes", got)
	}
}

func TestReconcileStillExpiresUploadWithBytesAfterResumeTTL(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 4, UploadResumeTTL: 24 * time.Hour, EmptyUploadTTL: 15 * time.Minute}, 4)
	f.createUpload(tusIDOne, 100)
	if response := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(jpegBody(10)), 10, f.cookie)); response.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d; want 204", response.Code)
	}
	ageUpload(t, f, tusIDOne, 25*time.Hour)
	report, err := f.store.Reconcile(t.Context())
	if err != nil || report.ExpiredResumable != 1 || len(f.tusFiles()) != 0 {
		t.Fatalf("Reconcile() = %+v, %v, files %v; want the upload expired", report, err, f.tusFiles())
	}
}

func TestDerivativeAtStagedCapAnswers503ButRetriesStillAnswer200(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MaxPendingUploads: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 2, time.Second, 1024, nil)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := []byte{0xff, 0xd8, 0xff, 1}
	post := func(id, variant string) int {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, derivativeRequest(id, variant, body, cookie))
		if response.Code == http.StatusServiceUnavailable {
			if response.Body.String() != tooManyStaged || response.Header().Get("Retry-After") != "30" {
				t.Fatalf("503 = %q, Retry-After %q; want too_many_staged_derivatives and 30", response.Body.String(), response.Header().Get("Retry-After"))
			}
		}
		return response.Code
	}
	const other = "223e4567-e89b-42d3-a456-426614174201"
	if post(testMediaID, "t") != http.StatusCreated || post(testMediaID, "d") != http.StatusCreated {
		t.Fatal("first two staged derivatives want 201")
	}
	before, err := os.ReadDir(paths.Tmp)
	if err != nil {
		t.Fatal(err)
	}
	if got := post(other, "t"); got != http.StatusServiceUnavailable {
		t.Fatalf("third staged derivative = %d; want 503", got)
	}
	after, err := os.ReadDir(paths.Tmp)
	if err != nil || len(after) != len(before) {
		t.Fatalf("tmp entries after rejection = %d, %v; want %d", len(after), err, len(before))
	}
	if got := post(testMediaID, "t"); got != http.StatusOK {
		t.Fatalf("retry of a staged derivative = %d; want 200", got)
	}
}
