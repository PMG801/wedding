package upload

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// tusSession returns the tus endpoint of store, the same one the HTTP handler mounts.
func tusSession(t *testing.T, store *Store) *Resumable {
	t.Helper()
	resumable, err := store.Resumable(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return resumable
}

// tusCreateUpload sends the tus creation (POST) of id with a declared size.
func tusCreateUpload(resumable *Resumable, id string, size int64) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/uploads/", nil)
	request.Header.Set("Tus-Resumable", "1.0.0")
	request.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	request.Header.Set("Upload-Metadata", "id "+base64.StdEncoding.EncodeToString([]byte(id))+",filetype "+base64.StdEncoding.EncodeToString([]byte("image/jpeg")))
	response := httptest.NewRecorder()
	resumable.ServeHTTP(response, request)
	return response
}

// tusPatchUpload sends bytes at offset of upload id (PATCH).
func tusPatchUpload(resumable *Resumable, id string, offset int64, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPatch, "/api/uploads/"+id, bytes.NewReader(body))
	request.Header.Set("Tus-Resumable", "1.0.0")
	request.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	request.Header.Set("Content-Type", "application/offset+octet-stream")
	response := httptest.NewRecorder()
	resumable.ServeHTTP(response, request)
	return response
}

// tusDeleteUpload cancels pending upload id (DELETE).
func tusDeleteUpload(resumable *Resumable, id string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, "/api/uploads/"+id, nil)
	request.Header.Set("Tus-Resumable", "1.0.0")
	response := httptest.NewRecorder()
	resumable.ServeHTTP(response, request)
	return response
}

// tusSeed uploads body as the original of id through tus: a creation that
// declares size, then one PATCH carrying body from offset 0. It returns the
// creation response when the creation is refused, else the PATCH response.
func tusSeed(t *testing.T, store *Store, id string, size int64, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	resumable := tusSession(t, store)
	created := tusCreateUpload(resumable, id, size)
	if created.Code != http.StatusCreated {
		return created
	}
	return tusPatchUpload(resumable, id, 0, body)
}

// seedOriginal publishes body as the original of id through tus and returns
// the confirmed medium. It fails the test unless the upload completes.
func seedOriginal(t *testing.T, store *Store, id string, body []byte) Media {
	t.Helper()
	if response := tusSeed(t, store, id, int64(len(body)), body); response.Code != http.StatusNoContent {
		t.Fatalf("tus upload of %s = %d %q; want 204", id, response.Code, response.Body.String())
	}
	media, err := store.confirmed(context.Background(), id)
	if err != nil {
		t.Fatalf("confirmed %s: %v", id, err)
	}
	return media
}
