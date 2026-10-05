package httpapi

import (
	"bytes"
	"context"
	"io"
	"os"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

const (
	testEventToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	testSessionKey = "0123456789abcdef0123456789abcdef"
)

func TestPhotoUploadRequiresGuestSessionAndStoresRawBodyIdempotently(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 2, time.Second, 1024)
	const id = "123e4567-e89b-42d3-a456-426614174000"
	body := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, photoRequest(id, body, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated PUT status = %d; want 401", unauthorized.Code)
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, photoRequest(id, body, auth.NewGuestCookie([]byte(testSessionKey), time.Now())))
	if first.Code != http.StatusCreated || first.Body.Len() != 0 {
		t.Fatalf("first PUT response = %d %q; want empty 201", first.Code, first.Body.String())
	}

	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, photoRequest(id, body, auth.NewGuestCookie([]byte(testSessionKey), time.Now())))
	if retry.Code != http.StatusOK || retry.Body.Len() != 0 {
		t.Fatalf("confirmed retry response = %d %q; want empty 200", retry.Code, retry.Body.String())
	}
}

func TestPhotoUploadValidatesIDLengthAndPhotoSignature(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 9, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 9)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	for _, test := range []struct {
		name           string
		id             string
		body           []byte
		wantStatus     int
		wantError      string
		declaredLength int64
	}{
		{"noncanonical UUID", "123E4567-e89b-42d3-a456-426614174000", []byte("x"), http.StatusBadRequest, "invalid_media_id", 0},
		{"missing length", "123e4567-e89b-42d3-a456-426614174001", nil, http.StatusLengthRequired, "content_length_required", 0},
		{"too large", "123e4567-e89b-42d3-a456-426614174002", bytes.Repeat([]byte("x"), 10), http.StatusRequestEntityTooLarge, "photo_too_large", 0},
		{"unsupported signature", "123e4567-e89b-42d3-a456-426614174003", []byte("not-photo"), http.StatusUnsupportedMediaType, "unsupported_photo_format", 0},
		{"truncated body", "123e4567-e89b-42d3-a456-426614174004", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, http.StatusBadRequest, "photo_body_size_mismatch", 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := photoRequest(test.id, test.body, cookie)
			if test.name == "missing length" {
				request.ContentLength = -1
			} else if test.declaredLength > 0 {
				request.ContentLength = test.declaredLength
			}
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Body.String() != "{\"error\":\""+test.wantError+"\"}\n" {
				t.Fatalf("PUT response = %d %q; want %d error %q", response.Code, response.Body.String(), test.wantStatus, test.wantError)
			}
		})
	}
}

func TestPhotoUploadMinimumFreeDiskReturnsInsufficientStorage(t *testing.T) {
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := upload.NewStore(database, paths, upload.Options{MaxBytes: 1024, MaxConcurrent: 1, MinFreeBytes: ^uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, photoRequest("123e4567-e89b-42d3-a456-426614174000", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, cookie))
	if response.Code != http.StatusInsufficientStorage || response.Body.String() != "{\"error\":\"insufficient_storage\"}\n" {
		t.Fatalf("PUT response = %d %q; want 507 insufficient_storage", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(paths.Originals)
	if err != nil || len(entries) != 0 {
		t.Fatalf("final files after rejected upload = %v, %v; want none", entries, err)
	}
}

func TestPhotoUploadRejectsSaturatedCapacityPromptly(t *testing.T) {
	photos := &blockingPhotoUploader{started: make(chan struct{}), release: make(chan struct{})}
	handler := NewHandler(testEventToken, []byte(testSessionKey), photos, 1, time.Second, 1024)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, photoRequest("123e4567-e89b-42d3-a456-426614174010", []byte("raw"), cookie))
	}()
	select {
	case <-photos.started:
	case <-time.After(time.Second):
		close(photos.release)
		t.Fatal("first upload did not enter the persistence service")
	}

	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, photoRequest("123e4567-e89b-42d3-a456-426614174011", []byte("raw"), cookie))
		secondDone <- response
	}()
	select {
	case response := <-secondDone:
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "1" || response.Body.String() != "{\"error\":\"upload_capacity_reached\"}\n" {
			t.Errorf("saturated PUT response = %d Retry-After=%q; want 503 and 1", response.Code, response.Header().Get("Retry-After"))
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("saturated PUT blocked instead of promptly returning 503")
	}
	close(photos.release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first upload did not finish after capacity was released")
	}
}

func photoRequest(id string, body []byte, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodPut, "/api/media/"+id, bytes.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func newHTTPPhotoStore(t *testing.T, options upload.Options) *upload.Store {
	t.Helper()
	root := t.TempDir()
	paths, err := storage.Initialize(root)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := upload.NewStore(database, paths, options)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type blockingPhotoUploader struct {
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func (u *blockingPhotoUploader) SavePhoto(ctx context.Context, id string, size int64, body io.Reader) (upload.Media, error) {
	u.startOnce.Do(func() { close(u.started) })
	select {
	case <-u.release:
		return upload.Media{ID: id}, nil
	case <-ctx.Done():
		return upload.Media{}, ctx.Err()
	}
}
