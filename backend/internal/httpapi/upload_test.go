package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestPhotoUploadIdleTimeoutOverHTTP(t *testing.T) {
	const idleTimeout = 150 * time.Millisecond
	handler := NewHandler(testEventToken, []byte(testSessionKey), drainingPhotoUploader{}, 1, idleTimeout, 1024)
	server := httptest.NewServer(handler)
	defer server.Close()

	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set client deadline: %v", err)
	}
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	_, err = io.WriteString(conn, "PUT /api/media/123e4567-e89b-42d3-a456-426614174020 HTTP/1.1\r\n"+
		"Host: "+server.Listener.Addr().String()+"\r\n"+
		"Cookie: "+cookie.Name+"="+cookie.Value+"\r\n"+
		"Content-Length: 10\r\nConnection: close\r\n\r\nabc")
	if err != nil {
		t.Fatalf("send partial upload request: %v", err)
	}

	started := time.Now()
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPut})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("read response after stalled upload: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("idle-timeout response took %s; want prompt response", elapsed)
	}
	if response.StatusCode != http.StatusRequestTimeout || string(responseBody) != "{\"error\":\"upload_idle_timeout\"}\n" {
		t.Fatalf("stalled PUT response = %d %q after %s; want 408 upload_idle_timeout", response.StatusCode, responseBody, elapsed)
	}

	normalConn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("connect for complete upload: %v", err)
	}
	defer normalConn.Close()
	if err := normalConn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set complete-upload client deadline: %v", err)
	}
	_, err = io.WriteString(normalConn, "PUT /api/media/123e4567-e89b-42d3-a456-426614174021 HTTP/1.1\r\n"+
		"Host: "+server.Listener.Addr().String()+"\r\n"+
		"Cookie: "+cookie.Name+"="+cookie.Value+"\r\n"+
		"Content-Length: 3\r\nConnection: close\r\n\r\nabc")
	if err != nil {
		t.Fatalf("send complete upload request: %v", err)
	}
	normalResponse, err := http.ReadResponse(bufio.NewReader(normalConn), &http.Request{Method: http.MethodPut})
	if err != nil {
		t.Fatalf("read complete-upload response: %v", err)
	}
	defer normalResponse.Body.Close()
	if normalResponse.StatusCode != http.StatusCreated {
		t.Fatalf("complete PUT status = %d; want 201", normalResponse.StatusCode)
	}
}

type drainingPhotoUploader struct{}

func (drainingPhotoUploader) SavePhoto(_ context.Context, id string, _ int64, body io.Reader) (upload.Media, error) {
	if _, err := io.Copy(io.Discard, body); err != nil {
		return upload.Media{}, err
	}
	return upload.Media{ID: id}, nil
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
