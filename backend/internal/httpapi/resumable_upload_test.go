package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

const (
	tusVersion        = "1.0.0"
	tusOffsetType     = "application/offset+octet-stream"
	tusIDOne          = "223e4567-e89b-42d3-a456-426614174101"
	tusIDTwo          = "223e4567-e89b-42d3-a456-426614174102"
	tusIDThree        = "223e4567-e89b-42d3-a456-426614174103"
	tusUnknownID      = "323e4567-e89b-42d3-a456-426614174999"
	tusGuestRequired  = "guest session required\n"
	tusCapacityReject = "{\"error\":\"upload_capacity_reached\"}\n"
)

// resumableFixture runs the real handler stack over a temporary data directory.
// build recreates the store and handler over the same directory, as a restart does.
type resumableFixture struct {
	t        *testing.T
	database *sql.DB
	paths    storage.Paths
	store    *upload.Store
	handler  http.Handler
	cookie   *http.Cookie
	switches *fakeUploadSwitch
}

type fakeUploadSwitch struct{ enabled atomic.Bool }

func (s *fakeUploadSwitch) UploadsEnabled() bool { return s.enabled.Load() }

func (s *fakeUploadSwitch) SetUploadsEnabled(_ context.Context, enabled bool) error {
	s.enabled.Store(enabled)
	return nil
}

func newResumableFixture(t *testing.T, options upload.Options, maxConcurrent int) *resumableFixture {
	t.Helper()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	f := &resumableFixture{
		t:        t,
		database: database,
		paths:    paths,
		cookie:   auth.NewGuestCookie([]byte(testSessionKey), time.Now()),
		switches: &fakeUploadSwitch{},
	}
	f.switches.enabled.Store(true)
	f.build(options, maxConcurrent)
	return f
}

func (f *resumableFixture) build(options upload.Options, maxConcurrent int) {
	f.t.Helper()
	store, err := upload.NewStore(f.database, f.paths, options)
	if err != nil {
		f.t.Fatal(err)
	}
	resumable, err := store.Resumable(time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	f.store = store
	f.handler = NewHandler(testEventToken, []byte(testSessionKey), store, maxConcurrent, time.Second, options.MaxBytes, nil,
		WithResumable(resumable),
		WithAdmin("", f.switches, store),
	)
}

func (f *resumableFixture) serve(request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *resumableFixture) createUpload(id string, length int64) {
	f.t.Helper()
	response := f.serve(tusCreate(idMetadata(id), length, f.cookie))
	if response.Code != http.StatusCreated {
		f.t.Fatalf("create upload %s = %d %q; want 201", id, response.Code, response.Body.String())
	}
}

func (f *resumableFixture) mediaRows() int {
	f.t.Helper()
	var count int
	if err := f.database.QueryRow("SELECT COUNT(*) FROM media").Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func (f *resumableFixture) names(dir string) []string {
	f.t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		f.t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func (f *resumableFixture) tusFiles() []string { return f.names(f.paths.Tus) }

func (f *resumableFixture) requireNoChanges(context string) {
	f.t.Helper()
	if rows := f.mediaRows(); rows != 0 {
		f.t.Fatalf("%s left %d media rows; want 0", context, rows)
	}
	if files := f.tusFiles(); len(files) != 0 {
		f.t.Fatalf("%s left tus files %v; want none", context, files)
	}
	if files := f.names(f.paths.Originals); len(files) != 0 {
		f.t.Fatalf("%s left originals %v; want none", context, files)
	}
}

func idMetadata(id string) string {
	return "id " + base64.StdEncoding.EncodeToString([]byte(id)) + ",filetype " + base64.StdEncoding.EncodeToString([]byte("image/jpeg"))
}

func tusCreate(metadata string, length int64, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/uploads/", nil)
	request.Header.Set("Tus-Resumable", tusVersion)
	request.Header.Set("Upload-Length", strconv.FormatInt(length, 10))
	if metadata != "" {
		request.Header.Set("Upload-Metadata", metadata)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func tusPatch(id string, offset int64, body io.Reader, length int64, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodPatch, "/api/uploads/"+id, body)
	request.ContentLength = length
	request.Header.Set("Tus-Resumable", tusVersion)
	request.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	request.Header.Set("Content-Type", tusOffsetType)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func tusHead(id string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodHead, "/api/uploads/"+id, nil)
	request.Header.Set("Tus-Resumable", tusVersion)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func tusDelete(id string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodDelete, "/api/uploads/"+id, nil)
	request.Header.Set("Tus-Resumable", tusVersion)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

// seedOriginalViaTus uploads body as the original of id through the public
// handler: a creation that declares the length, then one PATCH with every byte.
// It fails the test unless the PATCH completes the upload.
func seedOriginalViaTus(t *testing.T, handler http.Handler, id string, body []byte, cookie *http.Cookie) {
	t.Helper()
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, tusCreate(idMetadata(id), int64(len(body)), cookie))
	if created.Code != http.StatusCreated {
		t.Fatalf("create upload %s = %d %q; want 201", id, created.Code, created.Body.String())
	}
	patched := httptest.NewRecorder()
	handler.ServeHTTP(patched, tusPatch(id, 0, bytes.NewReader(body), int64(len(body)), cookie))
	if patched.Code != http.StatusNoContent {
		t.Fatalf("PATCH upload %s = %d %q; want 204", id, patched.Code, patched.Body.String())
	}
}

func assertOffset(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := response.Header().Get("Upload-Offset"); got != want {
		t.Fatalf("Upload-Offset = %q; want %q (status %d)", got, want, response.Code)
	}
}

func jpegBody(size int) []byte {
	body := make([]byte, size)
	copy(body, []byte{0xff, 0xd8, 0xff})
	for i := 3; i < size; i++ {
		body[i] = byte(i)
	}
	return body
}

// countingReader records reads so tests can prove a rejected request never touched its body.
type countingReader struct{ reads int }

func (r *countingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

// failingReader delivers data and then fails, like a connection cut mid-upload.
type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestResumableCreatePatchInTwoChunksPublishesOriginal(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	body := jpegBody(100)

	created := f.serve(tusCreate(idMetadata(tusIDOne), 100, f.cookie))
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %q; want 201", created.Code, created.Body.String())
	}
	if location := created.Header().Get("Location"); !strings.HasSuffix(location, "/api/uploads/"+tusIDOne) {
		t.Fatalf("Location = %q; want the upload URL for the media ID", location)
	}
	if got := created.Header().Get("Tus-Resumable"); got != tusVersion {
		t.Fatalf("Tus-Resumable = %q; want %s", got, tusVersion)
	}

	first := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(body[:40]), 40, f.cookie))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first PATCH = %d %q; want 204", first.Code, first.Body.String())
	}
	assertOffset(t, first, "40")
	if rows := f.mediaRows(); rows != 0 {
		t.Fatalf("media rows after partial upload = %d; want 0", rows)
	}

	second := f.serve(tusPatch(tusIDOne, 40, bytes.NewReader(body[40:]), 60, f.cookie))
	if second.Code != http.StatusNoContent {
		t.Fatalf("final PATCH = %d %q; want 204", second.Code, second.Body.String())
	}
	assertOffset(t, second, "100")
	if rows := f.mediaRows(); rows != 1 {
		t.Fatalf("media rows after finish = %d; want 1", rows)
	}
	var size int64
	var mime string
	if err := f.database.QueryRow("SELECT size_bytes, media_type FROM media WHERE id = ?", tusIDOne).Scan(&size, &mime); err != nil {
		t.Fatal(err)
	}
	if size != 100 || mime != "image/jpeg" {
		t.Fatalf("media row = size %d type %q; want 100 image/jpeg", size, mime)
	}
	originals := f.names(f.paths.Originals)
	if len(originals) != 1 || !strings.HasSuffix(originals[0], "_"+tusIDOne+".jpg") {
		t.Fatalf("originals = %v; want one chronologically named file for the media ID", originals)
	}
	if files := f.tusFiles(); len(files) != 0 {
		t.Fatalf("tmp/tus after publication = %v; want none", files)
	}
}

func TestResumableInterruptedPatchKeepsOffsetAndResumes(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	body := jpegBody(100)
	f.createUpload(tusIDOne, 100)

	cut := f.serve(tusPatch(tusIDOne, 0, &failingReader{data: body[:40], err: io.ErrUnexpectedEOF}, 100, f.cookie))
	if cut.Code == http.StatusNoContent {
		t.Fatalf("interrupted PATCH = 204; want a failure response")
	}
	if rows := f.mediaRows(); rows != 0 {
		t.Fatalf("interrupted upload confirmed media: %d rows", rows)
	}

	head := f.serve(tusHead(tusIDOne, f.cookie))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD after interruption = %d; want 200", head.Code)
	}
	assertOffset(t, head, "40")
	if got := head.Header().Get("Upload-Length"); got != "100" {
		t.Fatalf("Upload-Length after interruption = %q; want 100", got)
	}
	if got := head.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q; want no-store", got)
	}

	resumed := f.serve(tusPatch(tusIDOne, 40, bytes.NewReader(body[40:]), 60, f.cookie))
	if resumed.Code != http.StatusNoContent {
		t.Fatalf("resumed PATCH = %d %q; want 204", resumed.Code, resumed.Body.String())
	}
	assertOffset(t, resumed, "100")
	if rows := f.mediaRows(); rows != 1 {
		t.Fatalf("media rows after resume = %d; want 1", rows)
	}
}

func TestResumableConfirmedMediaHeadIsCompleteAndCreateConflicts(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	body := jpegBody(100)
	f.createUpload(tusIDOne, 100)
	if response := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(body), 100, f.cookie)); response.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %q; want 204", response.Code, response.Body.String())
	}

	head := f.serve(tusHead(tusIDOne, f.cookie))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD confirmed = %d; want 200", head.Code)
	}
	assertOffset(t, head, "100")
	if got := head.Header().Get("Upload-Length"); got != "100" {
		t.Fatalf("HEAD confirmed Upload-Length = %q; want 100", got)
	}
	if got := head.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("HEAD confirmed Cache-Control = %q; want no-store", got)
	}
	if got := head.Header().Get("Tus-Resumable"); got != tusVersion {
		t.Fatalf("HEAD confirmed Tus-Resumable = %q; want %s", got, tusVersion)
	}

	conflict := f.serve(tusCreate(idMetadata(tusIDOne), 100, f.cookie))
	if conflict.Code != http.StatusConflict || conflict.Body.String() != "{\"error\":\"already_confirmed\"}\n" {
		t.Fatalf("POST confirmed = %d %q; want 409 already_confirmed", conflict.Code, conflict.Body.String())
	}
	if rows := f.mediaRows(); rows != 1 {
		t.Fatalf("media rows after conflict = %d; want 1", rows)
	}
}

func TestResumableRejectsInvalidMediaIDWithoutChangingState(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	for name, metadata := range map[string]string{
		"uppercase UUID": "id " + base64.StdEncoding.EncodeToString([]byte("223E4567-E89B-42D3-A456-426614174101")),
		"missing id":     "filetype " + base64.StdEncoding.EncodeToString([]byte("image/jpeg")),
		"not a UUID":     "id " + base64.StdEncoding.EncodeToString([]byte("../etc/passwd")),
	} {
		response := f.serve(tusCreate(metadata, 100, f.cookie))
		if response.Code != http.StatusBadRequest || response.Body.String() != "{\"error\":\"invalid_media_id\"}\n" {
			t.Fatalf("%s: POST = %d %q; want 400 invalid_media_id", name, response.Code, response.Body.String())
		}
	}
	f.requireNoChanges("invalid media IDs")
}

func TestResumableRejectsTooLargeUploadBeforeStorage(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 40, MaxVideoBytes: 200, MaxConcurrent: 2}, 2)
	response := f.serve(tusCreate(idMetadata(tusIDOne), 201, f.cookie))
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"media_too_large\"}\n" {
		t.Fatalf("POST above every limit = %d %q; want 413 media_too_large", response.Code, response.Body.String())
	}
	f.requireNoChanges("oversized upload")
}

func TestResumableCreateReservesPendingBytesBeforeInsufficientStorage(t *testing.T) {
	free := uint64(1000)
	options := upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MinFreeBytes: 100, FreeSpace: func(string) (uint64, error) { return free, nil }}
	f := newResumableFixture(t, options, 2)

	f.createUpload(tusIDOne, 500)
	// 1000 free - 500 pending - 450 declared leaves 50, below the 100-byte minimum.
	rejected := f.serve(tusCreate(idMetadata(tusIDTwo), 450, f.cookie))
	if rejected.Code != http.StatusInsufficientStorage || rejected.Body.String() != "{\"error\":\"insufficient_storage\"}\n" {
		t.Fatalf("POST over reservation = %d %q; want 507 insufficient_storage", rejected.Code, rejected.Body.String())
	}
	if files := f.tusFiles(); len(files) != 2 || files[0] != tusIDOne || files[1] != tusIDOne+".info" {
		t.Fatalf("tus files after rejected create = %v; want only the first upload", files)
	}

	f.createUpload(tusIDThree, 300)
	if files := f.tusFiles(); len(files) != 4 {
		t.Fatalf("tus files after admitted create = %v; want two uploads", files)
	}
}

func TestResumableRejectsUnsupportedSignatureAtFinishAndDeletesUpload(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.createUpload(tusIDOne, 9)

	response := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader([]byte("not-photo")), 9, f.cookie))
	if response.Code != http.StatusUnsupportedMediaType || response.Body.String() != "{\"error\":\"unsupported_photo_format\"}\n" {
		t.Fatalf("PATCH with bad signature = %d %q; want 415 unsupported_photo_format", response.Code, response.Body.String())
	}
	if head := f.serve(tusHead(tusIDOne, f.cookie)); head.Code != http.StatusNotFound {
		t.Fatalf("HEAD after rejected upload = %d; want 404", head.Code)
	}
	f.requireNoChanges("unsupported signature")
}

func TestResumableRejectsPhotoAboveLimitAtFinishAndDeletesUpload(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 40, MaxVideoBytes: 200, MaxConcurrent: 2}, 2)
	body := jpegBody(60)
	f.createUpload(tusIDOne, 60)

	response := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(body), 60, f.cookie))
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"photo_too_large\"}\n" {
		t.Fatalf("PATCH photo above photo limit = %d %q; want 413 photo_too_large", response.Code, response.Body.String())
	}
	f.requireNoChanges("photo above limit")
}

func TestResumablePausedSwitchRejectsPostAndPatchWithoutReadingBody(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.createUpload(tusIDOne, 100)
	f.switches.enabled.Store(false)

	body := &countingReader{}
	patch := f.serve(tusPatch(tusIDOne, 0, body, 100, f.cookie))
	if patch.Code != http.StatusServiceUnavailable || patch.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("PATCH while paused = %d %q; want 503 uploads_paused", patch.Code, patch.Body.String())
	}
	if patch.Header().Get("Retry-After") != "" {
		t.Fatalf("paused PATCH sent Retry-After; want none")
	}
	if body.reads != 0 {
		t.Fatalf("paused PATCH read its body %d times; want 0", body.reads)
	}

	create := f.serve(tusCreate(idMetadata(tusIDTwo), 100, f.cookie))
	if create.Code != http.StatusServiceUnavailable || create.Body.String() != "{\"error\":\"uploads_paused\"}\n" {
		t.Fatalf("POST while paused = %d %q; want 503 uploads_paused", create.Code, create.Body.String())
	}

	head := f.serve(tusHead(tusIDOne, f.cookie))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD while paused = %d; want reads to keep working", head.Code)
	}
	assertOffset(t, head, "0")
	if files := f.tusFiles(); len(files) != 2 {
		t.Fatalf("tus files while paused = %v; want only the existing upload", files)
	}
}

func TestResumableRequiresGuestSession(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.createUpload(tusIDOne, 100)
	body := &countingReader{}
	for name, request := range map[string]*http.Request{
		"POST":   tusCreate(idMetadata(tusIDTwo), 100, nil),
		"HEAD":   tusHead(tusIDOne, nil),
		"PATCH":  tusPatch(tusIDOne, 0, body, 100, nil),
		"DELETE": tusDelete(tusIDOne, nil),
	} {
		response := f.serve(request)
		if response.Code != http.StatusUnauthorized || response.Body.String() != tusGuestRequired {
			t.Fatalf("%s without guest cookie = %d %q; want 401", name, response.Code, response.Body.String())
		}
	}
	if body.reads != 0 {
		t.Fatalf("unauthenticated PATCH read its body")
	}
	if files := f.tusFiles(); len(files) != 2 {
		t.Fatalf("tus files after unauthenticated requests = %v; want unchanged", files)
	}
}

func TestResumableCapacityReachedRejectsPatchWithoutReadingBody(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 1}, 1)
	body := jpegBody(100)
	f.createUpload(tusIDOne, 100)
	f.createUpload(tusIDTwo, 100)

	reader, writer := io.Pipe()
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- f.serve(tusPatch(tusIDOne, 0, reader, 100, f.cookie)) }()
	if _, err := writer.Write(body[:10]); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}

	second := &countingReader{}
	rejected := make(chan *httptest.ResponseRecorder, 1)
	go func() { rejected <- f.serve(tusPatch(tusIDTwo, 0, second, 100, f.cookie)) }()
	select {
	case response := <-rejected:
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "1" || response.Body.String() != tusCapacityReject {
			t.Fatalf("PATCH at capacity = %d Retry-After=%q %q; want 503 with Retry-After 1", response.Code, response.Header().Get("Retry-After"), response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("PATCH at capacity blocked instead of returning 503 promptly")
	}
	if second.reads != 0 {
		t.Fatalf("PATCH at capacity read its body %d times; want 0", second.reads)
	}

	_ = writer.CloseWithError(io.ErrUnexpectedEOF)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted PATCH did not finish")
	}
	head := f.serve(tusHead(tusIDOne, f.cookie))
	assertOffset(t, head, "10")
}

func TestResumableFinishPublishFailureKeepsCompleteUploadForRetry(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	body := jpegBody(100)
	f.createUpload(tusIDOne, 100)
	if _, err := f.database.Exec(`CREATE TRIGGER fail_media_insert BEFORE INSERT ON media BEGIN SELECT RAISE(ABORT, 'forced'); END;`); err != nil {
		t.Fatal(err)
	}

	failed := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(body), 100, f.cookie))
	if failed.Code != http.StatusInternalServerError || failed.Body.String() != "{\"error\":\"upload_failed\"}\n" {
		t.Fatalf("PATCH with failing database = %d %q; want 500 upload_failed", failed.Code, failed.Body.String())
	}
	if files := f.tusFiles(); len(files) != 2 {
		t.Fatalf("tus files after failed publication = %v; want the complete upload kept", files)
	}
	if originals := f.names(f.paths.Originals); len(originals) != 0 {
		t.Fatalf("originals after failed publication = %v; want none", originals)
	}

	if _, err := f.database.Exec(`DROP TRIGGER fail_media_insert`); err != nil {
		t.Fatal(err)
	}
	head := f.serve(tusHead(tusIDOne, f.cookie))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD retry = %d; want 200 after publication succeeds", head.Code)
	}
	assertOffset(t, head, "100")
	if rows := f.mediaRows(); rows != 1 {
		t.Fatalf("media rows after retry = %d; want 1", rows)
	}
	if files := f.tusFiles(); len(files) != 0 {
		t.Fatalf("tus files after retry = %v; want none", files)
	}
}

func TestResumableDeleteCancelsPendingUpload(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.createUpload(tusIDOne, 100)
	if response := f.serve(tusPatch(tusIDOne, 0, bytes.NewReader(jpegBody(10)), 10, f.cookie)); response.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d; want 204", response.Code)
	}

	if response := f.serve(tusDelete(tusIDOne, f.cookie)); response.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d; want 204", response.Code)
	}
	if files := f.tusFiles(); len(files) != 0 {
		t.Fatalf("tus files after DELETE = %v; want none", files)
	}
	if response := f.serve(tusHead(tusIDOne, f.cookie)); response.Code != http.StatusNotFound {
		t.Fatalf("HEAD after DELETE = %d; want 404", response.Code)
	}
	if response := f.serve(tusHead(tusUnknownID, f.cookie)); response.Code != http.StatusNotFound {
		t.Fatalf("HEAD unknown upload = %d; want 404", response.Code)
	}
}

func TestResumableReconcileExpiresStaleUploadsButKeepsFreshAndActiveOnes(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 4, UploadResumeTTL: time.Hour}, 4)
	f.createUpload(tusIDOne, 100) // stale and idle
	f.createUpload(tusIDTwo, 100) // fresh
	f.createUpload(tusIDThree, 100)
	reader, writer := io.Pipe()
	activeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { activeDone <- f.serve(tusPatch(tusIDThree, 0, reader, 100, f.cookie)) }()
	if _, err := writer.Write(jpegBody(10)); err != nil { // PATCH in flight, idle but holding the upload
		t.Fatal(err)
	}

	old := time.Now().Add(-2 * time.Hour)
	for _, id := range []string{tusIDOne, tusIDThree} {
		for _, name := range []string{id, id + ".info"} {
			if err := os.Chtimes(filepath.Join(f.paths.Tus, name), old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	report, err := f.store.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.ExpiredResumable != 1 {
		t.Fatalf("ExpiredResumable = %d; want 1", report.ExpiredResumable)
	}
	if files := f.tusFiles(); len(files) != 4 {
		t.Fatalf("tus files after reconcile = %v; want the fresh and active uploads", files)
	}
	for _, name := range []string{tusIDOne, tusIDOne + ".info"} {
		if _, err := os.Stat(filepath.Join(f.paths.Tus, name)); !os.IsNotExist(err) {
			t.Fatalf("expired upload file %s still present: %v", name, err)
		}
	}
	if info, err := os.Stat(f.paths.Tus); err != nil || !info.IsDir() {
		t.Fatalf("reconcile removed or moved tmp/tus: %v", err)
	}
	if quarantined := f.names(f.paths.Quarantine); len(quarantined) != 0 {
		t.Fatalf("reconcile quarantined %v; want none", quarantined)
	}

	_ = writer.CloseWithError(io.ErrUnexpectedEOF)
	<-activeDone
}

func TestResumableRestartStillCountsPendingReservation(t *testing.T) {
	free := uint64(1000)
	options := upload.Options{MaxBytes: 1024, MaxConcurrent: 2, MinFreeBytes: 100, FreeSpace: func(string) (uint64, error) { return free, nil }}
	f := newResumableFixture(t, options, 2)
	f.createUpload(tusIDOne, 500)

	f.build(options, 2) // a new process over the same data directory
	usage, err := f.store.DiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage.ReservedBytes != 500 {
		t.Fatalf("ReservedBytes after restart = %d; want the 500 pending bytes", usage.ReservedBytes)
	}
	rejected := f.serve(tusCreate(idMetadata(tusIDTwo), 450, f.cookie))
	if rejected.Code != http.StatusInsufficientStorage {
		t.Fatalf("POST after restart = %d %q; want 507", rejected.Code, rejected.Body.String())
	}
}

// The direct PUT upload is retired once tus carries originals (ADR-0014): the
// path must not accept a body, write a file or confirm media.
func TestRetiredPutMediaRouteWritesNothing(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	request := httptest.NewRequest(http.MethodPut, "/api/media/"+testMediaID, bytes.NewReader(jpegBody(64)))
	request.AddCookie(f.cookie)
	response := f.serve(request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("PUT /api/media/{id} = %d %q; want 404 from the mux (route retired)", response.Code, response.Body.String())
	}
	f.requireNoChanges("retired PUT route")
}

// readDirWithoutTus lists a directory for the cleanup assertions. tmp/tus is a
// managed directory of resumable uploads (ADR-0014), covered by its own tests,
// so it is not a leftover of a rejected photo or derivative.
func readDirWithoutTus(t *testing.T, directory string) ([]os.DirEntry, error) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	kept := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != "tus" {
			kept = append(kept, entry)
		}
	}
	return kept, err
}
