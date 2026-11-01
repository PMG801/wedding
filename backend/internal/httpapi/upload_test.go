package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
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
	testMediaID    = "123e4567-e89b-42d3-a456-426614174000"
)

func TestDerivativeUploadRequiresGuestSessionAndStoresRawJPEGIdempotently(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 2, time.Second, 1024, nil)
	const id = "123e4567-e89b-42d3-a456-426614174000"
	body := []byte{0xff, 0xd8, 0xff, 1, 2, 3}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, derivativeRequest(id, "t", body, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated derivative POST status = %d; want 401", unauthorized.Code)
	}

	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, derivativeRequest(id, "t", body, cookie))
	if first.Code != http.StatusCreated || first.Body.Len() != 0 {
		t.Fatalf("first derivative POST response = %d %q; want empty 201", first.Code, first.Body.String())
	}

	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, derivativeRequest(id, "t", body, cookie))
	if retry.Code != http.StatusOK || retry.Body.Len() != 0 {
		t.Fatalf("derivative retry response = %d %q; want empty 200", retry.Code, retry.Body.String())
	}
}

func TestDerivativeUploadRejectsInvalidInputsWithoutWritingFiles(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 5, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 5, nil)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	for _, test := range []struct {
		name       string
		variant    string
		body       []byte
		length     int64
		setLength  bool
		wantStatus int
		wantError  string
	}{
		{"invalid variant", "x", jpeg[:3], 0, false, http.StatusBadRequest, "invalid_derivative_variant"},
		{"empty body", "t", nil, 0, false, http.StatusBadRequest, "empty_derivative"},
		{"missing length", "t", jpeg[:3], -1, true, http.StatusLengthRequired, "content_length_required"},
		{"too large", "t", jpeg, 0, false, http.StatusRequestEntityTooLarge, "derivative_too_large"},
		{"unsupported signature", "t", []byte("bad"), 0, false, http.StatusUnsupportedMediaType, "unsupported_derivative_format"},
		{"truncated body", "t", jpeg[:3], 4, true, http.StatusBadRequest, "derivative_body_size_mismatch"},
		{"extra body bytes", "t", jpeg, 5, true, http.StatusBadRequest, "derivative_body_size_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := derivativeRequest(testMediaID, test.variant, test.body, cookie)
			if test.setLength {
				request.ContentLength = test.length
			}
			handler.ServeHTTP(response, request)
			wantBody := "{\"error\":\"" + test.wantError + "\"}\n"
			if response.Code != test.wantStatus || response.Body.String() != wantBody {
				t.Fatalf("derivative POST response = %d %q; want %d %q", response.Code, response.Body.String(), test.wantStatus, wantBody)
			}
			for _, directory := range []string{paths.Tmp, paths.Derived} {
				entries, err := readDirWithoutTus(t, directory)
				if err != nil || len(entries) != 0 {
					t.Fatalf("rejected derivative left files in %s: %v, %v", directory, entries, err)
				}
			}
		})
	}
}

func TestDerivativeUploadRequiresJPEGContentType(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, nil)
	request := derivativeRequest(testMediaID, "t", []byte{0xff, 0xd8, 0xff}, auth.NewGuestCookie([]byte(testSessionKey), time.Now()))
	request.Header.Set("Content-Type", "application/octet-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType || response.Body.String() != "{\"error\":\"unsupported_derivative_format\"}\n" {
		t.Fatalf("derivative with non-JPEG content type = %d %q; want 415", response.Code, response.Body.String())
	}
	for _, directory := range []string{paths.Tmp, paths.Derived} {
		entries, err := readDirWithoutTus(t, directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("rejected content type left files in %s: %v, %v", directory, entries, err)
		}
	}
}

func TestDerivativeUploadMinimumFreeDiskReturnsInsufficientStorage(t *testing.T) {
	store, paths := newHTTPPhotoStoreWithPaths(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 1, MinFreeBytes: ^uint64(0)})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, nil)
	request := derivativeRequest(testMediaID, "t", []byte{0xff, 0xd8, 0xff}, auth.NewGuestCookie([]byte(testSessionKey), time.Now()))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInsufficientStorage || response.Body.String() != "{\"error\":\"insufficient_storage\"}\n" {
		t.Fatalf("derivative under free-disk threshold = %d %q; want 507", response.Code, response.Body.String())
	}
	for _, directory := range []string{paths.Tmp, paths.Derived} {
		entries, err := readDirWithoutTus(t, directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("rejected derivative left files in %s: %v, %v", directory, entries, err)
		}
	}
}

// A tus PATCH that is still receiving bytes holds the only shared slot, so a
// derivative is refused promptly until the PATCH releases it.
func TestDerivativeUploadSharesCapacityWithOriginalUploads(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 1}, 1)
	f.createUpload(tusIDOne, 100)
	reader, writer := io.Pipe()
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- f.serve(tusPatch(tusIDOne, 0, reader, 100, f.cookie)) }()
	if _, err := writer.Write(jpegBody(10)); err != nil { // the PATCH reads, so it holds the slot
		t.Fatalf("write first chunk: %v", err)
	}

	second := f.serve(derivativeRequest(testMediaID, "t", []byte{0xff, 0xd8, 0xff}, f.cookie))
	if second.Code != http.StatusServiceUnavailable || second.Header().Get("Retry-After") != "1" || second.Body.String() != tusCapacityReject {
		t.Errorf("derivative during original upload = %d Retry-After=%q %q; want prompt 503", second.Code, second.Header().Get("Retry-After"), second.Body.String())
	}

	if _, err := writer.Write(jpegBody(90)); err != nil {
		t.Fatalf("write remaining bytes: %v", err)
	}
	_ = writer.Close() // the body ends at its declared length, as a client's request does
	select {
	case response := <-firstDone:
		if response.Code != http.StatusNoContent {
			t.Errorf("original upload after completing = %d %q; want 204", response.Code, response.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("original upload did not finish")
	}
}

// A derivative body that stalls mid-upload is answered 408 once the idle timeout
// passes, and a complete body on the same handler still succeeds.
func TestDerivativeUploadIdleTimeoutOverHTTP(t *testing.T) {
	const idleTimeout = 150 * time.Millisecond
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 2, idleTimeout, 1024, nil)
	server := httptest.NewServer(handler)
	defer server.Close()
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())

	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set client deadline: %v", err)
	}
	_, err = io.WriteString(conn, "POST /api/media/"+testMediaID+"/derived?variant=t HTTP/1.1\r\n"+
		"Host: "+server.Listener.Addr().String()+"\r\n"+
		"Cookie: "+cookie.Name+"="+cookie.Value+"\r\n"+
		"Content-Type: image/jpeg\r\n"+
		"Content-Length: 10\r\nConnection: close\r\n\r\n\xff\xd8\xffabc")
	if err != nil {
		t.Fatalf("send partial derivative request: %v", err)
	}

	started := time.Now()
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPost})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("read response after stalled derivative: %v", err)
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
		t.Fatalf("stalled derivative response = %d %q after %s; want 408 upload_idle_timeout", response.StatusCode, responseBody, elapsed)
	}

	normalConn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("connect for complete derivative: %v", err)
	}
	defer normalConn.Close()
	if err := normalConn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set complete-derivative client deadline: %v", err)
	}
	_, err = io.WriteString(normalConn, "POST /api/media/223e4567-e89b-42d3-a456-426614174021/derived?variant=t HTTP/1.1\r\n"+
		"Host: "+server.Listener.Addr().String()+"\r\n"+
		"Cookie: "+cookie.Name+"="+cookie.Value+"\r\n"+
		"Content-Type: image/jpeg\r\n"+
		"Content-Length: 6\r\nConnection: close\r\n\r\n\xff\xd8\xffabc")
	if err != nil {
		t.Fatalf("send complete derivative request: %v", err)
	}
	normalResponse, err := http.ReadResponse(bufio.NewReader(normalConn), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("read complete-derivative response: %v", err)
	}
	defer normalResponse.Body.Close()
	if normalResponse.StatusCode != http.StatusCreated {
		t.Fatalf("complete derivative status = %d; want 201", normalResponse.StatusCode)
	}
}

func derivativeRequest(id, variant string, body []byte, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/media/"+id+"/derived?variant="+variant, bytes.NewReader(body))
	request.Header.Set("Content-Type", "image/jpeg")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func newHTTPPhotoStore(t *testing.T, options upload.Options) *upload.Store {
	t.Helper()
	store, _ := newHTTPPhotoStoreWithPaths(t, options)
	return store
}

func newHTTPPhotoStoreWithPaths(t *testing.T, options upload.Options) (*upload.Store, storage.Paths) {
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
	return store, paths
}

// blockingPhotoDerivativeUploader holds a capacity slot in SaveDerivative until released.
type blockingPhotoDerivativeUploader struct {
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func (u *blockingPhotoDerivativeUploader) SaveDerivative(ctx context.Context, id, variant string, _ int64, _ io.Reader) (upload.Derivative, error) {
	u.startOnce.Do(func() { close(u.started) })
	select {
	case <-u.release:
		return upload.Derivative{}, nil
	case <-ctx.Done():
		return upload.Derivative{}, ctx.Err()
	}
}

func TestDerivativeUploadKeepsPhotoLimitWhenVideoLimitIsLarger(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 5, MaxVideoBytes: 200, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 5, nil)
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, derivativeRequest(testMediaID, "t", []byte{0xff, 0xd8, 0xff, 1, 2, 3}, cookie))
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"derivative_too_large\"}\n" {
		t.Fatalf("derivative above photo limit = %d %q", response.Code, response.Body.String())
	}
}
