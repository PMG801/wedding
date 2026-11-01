package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// readCountingBody counts the bytes the handler actually consumes from the request body.
type readCountingBody struct {
	io.ReadCloser
	consumed *atomic.Int64
}

func (b readCountingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.consumed.Add(int64(n))
	return n, err
}

// newRetryDrainServer serves handler over real HTTP and reports how many body
// bytes the handler consumed for each request.
func newRetryDrainServer(t *testing.T, handler http.Handler) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var consumed atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consumed.Store(0)
		r.Body = readCountingBody{ReadCloser: r.Body, consumed: &consumed}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &consumed
}

// sendRetry performs one request on client and reports its status and whether
// it reused a kept-alive connection.
func sendRetry(t *testing.T, client *http.Client, method, url string, body []byte, cookie *http.Cookie) (int, bool) {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "image/jpeg")
	}
	reused := false
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode, reused
}

func TestConfirmedDerivativeRetryDrainsDeclaredBodyAndKeepsConnectionAlive(t *testing.T) {
	const size = 1 << 20
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 2 << 20, MaxConcurrent: 2})
	resumable, err := store.Resumable(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 2, 5*time.Second, 2<<20, nil, WithResumable(resumable))
	server, consumed := newRetryDrainServer(t, handler)
	client := server.Client()
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	const id = "123e4567-e89b-42d3-a456-426614174040"
	photo := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 64)...)
	seedOriginalViaTus(t, handler, id, photo, cookie)
	derivative := append([]byte{0xff, 0xd8, 0xff}, make([]byte, size-3)...)
	url := server.URL + "/api/media/" + id + "/derived?variant=t"
	if status, _ := sendRetry(t, client, http.MethodPost, url, derivative, cookie); status != http.StatusCreated {
		t.Fatalf("first derivative POST = %d; want 201", status)
	}

	status, reused := sendRetry(t, client, http.MethodPost, url, derivative, cookie)
	if status != http.StatusOK {
		t.Fatalf("confirmed derivative re-POST of 1 MiB = %d; want 200", status)
	}
	if got := consumed.Load(); got != size {
		t.Fatalf("confirmed derivative re-POST consumed %d body bytes; want the whole %d-byte body drained", got, size)
	}
	if !reused {
		t.Fatal("derivative re-POST opened a new connection; want the kept-alive connection reused")
	}
}
