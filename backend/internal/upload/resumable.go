package upload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tus/tusd/v2/pkg/filestore"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"
	expslog "golang.org/x/exp/slog"
)

// resumableBasePath is where tus 1.0.0 uploads are served (ADR-0014).
const resumableBasePath = "/api/uploads/"

// ErrResumableNotFound reports that neither a pending upload nor a confirmed
// medium exists for an ID.
var ErrResumableNotFound = errors.New("resumable upload not found")

// ResumableStatus is the progress a HEAD request reports.
type ResumableStatus struct {
	// Offset is the number of bytes already stored, Length the declared size.
	Offset, Length int64
	// Complete is true once the medium is confirmed.
	Complete bool
}

// Resumable serves tus 1.0.0 for original media (ADR-0014). Uploads are stored
// under tmp/tus with the media ID as their upload ID.
type Resumable struct {
	store *Store
	tus   *handler.Handler
}

// resumableCore is the tusd data store. It guards creation so that the duplicate
// check, the disk reservation and the creation share the ledger mutex.
type resumableCore struct {
	filestore.FileStore
	store *Store
}

// Resumable builds the tus endpoint. idleTimeout bounds how long a PATCH body may
// stall before tusd gives up on it.
func (s *Store) Resumable(idleTimeout time.Duration) (*Resumable, error) {
	if idleTimeout <= 0 {
		return nil, errors.New("positive upload idle timeout is required")
	}
	core := resumableCore{FileStore: filestore.New(s.paths.Tus), store: s}
	composer := handler.NewStoreComposer()
	composer.UseCore(core)
	composer.UseTerminater(core)
	// Serializes requests on one upload, so two PATCHes never write the same offset.
	composer.UseLocker(memorylocker.New())
	tus, err := handler.NewHandler(handler.Config{
		BasePath:                  resumableBasePath,
		StoreComposer:             composer,
		NetworkTimeout:            idleTimeout,
		DisableDownload:           true,
		DisableConcatenation:      true,
		Cors:                      &handler.CorsConfig{Disable: true},
		PreUploadCreateCallback:   s.resumableCreateHook,
		PreFinishResponseCallback: s.resumableFinishHook,
		// tusd logs every request at info; only its warnings matter here.
		Logger: tusLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("configure tus handler: %w", err)
	}
	return &Resumable{store: s, tus: tus}, nil
}

// ServeHTTP serves the tus protocol under /api/uploads/. tusd routes on the
// whole path, so the prefix is removed from a copy of the request first.
// PATCH requests are counted as in flight so that reconciliation never expires
// an upload that is receiving bytes.
func (r *Resumable) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPatch {
		id := strings.TrimPrefix(req.URL.Path, resumableBasePath)
		if ValidID(id) {
			done := r.store.beginPatch(id)
			defer done()
		}
	}
	stripped := *req.URL
	stripped.Path = strings.TrimPrefix(req.URL.Path, strings.TrimSuffix(resumableBasePath, "/"))
	stripped.RawPath = ""
	inner := req.WithContext(req.Context())
	inner.URL = &stripped
	r.tus.ServeHTTP(relativeLocationWriter{ResponseWriter: w}, inner)
}

// relativeLocationWriter turns the absolute Location that tusd builds from the
// request into the path-only URL that docs/api.md documents.
type relativeLocationWriter struct{ http.ResponseWriter }

func (w relativeLocationWriter) WriteHeader(status int) {
	if location, err := url.Parse(w.Header().Get("Location")); err == nil && location.IsAbs() {
		w.Header().Set("Location", location.RequestURI())
	}
	w.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the server's writer, where tusd sets deadlines.
func (w relativeLocationWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Head reports the progress of upload id. A medium that is confirmed reports as
// complete. An upload whose bytes are all on disk but not yet published is
// published now, so a retry after a failed publication completes the upload.
func (r *Resumable) Head(ctx context.Context, id string) (ResumableStatus, error) {
	s := r.store
	if !ValidID(id) {
		return ResumableStatus{}, ErrResumableNotFound
	}
	if existing, err := s.confirmed(ctx, id); err == nil {
		return ResumableStatus{Offset: existing.SizeBytes, Length: existing.SizeBytes, Complete: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ResumableStatus{}, fmt.Errorf("check existing media: %w", err)
	}
	info, stored, err := s.readPending(id)
	if err != nil {
		return ResumableStatus{}, err
	}
	if stored < info.Size {
		return ResumableStatus{Offset: stored, Length: info.Size}, nil
	}

	unlock, err := s.lockID(ctx, id)
	if err != nil {
		return ResumableStatus{}, err
	}
	defer unlock()
	media, err := s.finishResumable(ctx, id)
	if err != nil {
		return ResumableStatus{}, err
	}
	return ResumableStatus{Offset: media.SizeBytes, Length: media.SizeBytes, Complete: true}, nil
}

// PublishErrorStatus maps a publication failure to its HTTP status and error
// code. Only signature and size failures reject the upload; the rest keep it.
func PublishErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrUnsupportedPhoto):
		return http.StatusUnsupportedMediaType, "unsupported_photo_format"
	case errors.Is(err, ErrPhotoTooLarge):
		return http.StatusRequestEntityTooLarge, "photo_too_large"
	case errors.Is(err, ErrVideoTooLarge):
		return http.StatusRequestEntityTooLarge, "video_too_large"
	default:
		return http.StatusInternalServerError, "upload_failed"
	}
}

// NewUpload admits the upload under the ledger mutex: it refuses an ID that is
// already pending, then checks the free space against every reservation.
func (c resumableCore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	s := c.store
	if !ValidID(info.ID) {
		return nil, jsonHTTPError(http.StatusBadRequest, "invalid_media_id")
	}
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	if _, err := os.Stat(s.resumableInfoPath(info.ID)); err == nil {
		return nil, jsonHTTPError(http.StatusConflict, "upload_in_progress")
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("check pending upload: %w", err)
	}
	if err := s.admitLocked(info.Size); err != nil {
		if errors.Is(err, ErrInsufficientStorage) {
			return nil, jsonHTTPError(http.StatusInsufficientStorage, "insufficient_storage")
		}
		return nil, err
	}
	return c.FileStore.NewUpload(ctx, info)
}

// resumableCreateHook rejects creations that can never succeed before any file exists.
func (s *Store) resumableCreateHook(hook handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error) {
	id := hook.Upload.MetaData["id"]
	if !ValidID(id) {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, jsonHTTPError(http.StatusBadRequest, "invalid_media_id")
	}
	if hook.Upload.Size <= 0 {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, jsonHTTPError(http.StatusBadRequest, "invalid_upload_length")
	}
	if _, err := s.confirmed(hook.Context, id); err == nil {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, jsonHTTPError(http.StatusConflict, "already_confirmed")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, fmt.Errorf("check existing media: %w", err)
	}
	if hook.Upload.Size > s.maxMediaBytes() {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, jsonHTTPError(http.StatusRequestEntityTooLarge, "media_too_large")
	}
	return handler.HTTPResponse{}, handler.FileInfoChanges{ID: id}, nil
}

// resumableFinishHook publishes the upload when its last byte arrives. Errors
// reach the client as JSON; a complete upload is kept unless its content is rejected.
func (s *Store) resumableFinishHook(hook handler.HookEvent) (handler.HTTPResponse, error) {
	unlock, err := s.lockID(hook.Context, hook.Upload.ID)
	if err != nil {
		return handler.HTTPResponse{}, err
	}
	defer unlock()
	if _, err := s.finishResumable(hook.Context, hook.Upload.ID); err != nil {
		status, code := PublishErrorStatus(err)
		return handler.HTTPResponse{}, jsonHTTPError(status, code)
	}
	return handler.HTTPResponse{}, nil
}

// finishResumable publishes the complete pending upload id. The caller holds the ID lock.
func (s *Store) finishResumable(ctx context.Context, id string) (Media, error) {
	if existing, err := s.confirmed(ctx, id); err == nil {
		// Confirmed by an earlier attempt; only leftovers of that attempt remain.
		s.removeResumable(id)
		if existing.Visibility == "visible" {
			s.promotePendingDerivatives(id)
		}
		existing.AlreadyConfirmed = true
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Media{}, fmt.Errorf("check existing media: %w", err)
	}
	info, _, err := s.readPending(id)
	if err != nil {
		return Media{}, err
	}
	media, err := s.publishPart(ctx, id, s.resumableBinPath(id), info.Size)
	if err != nil {
		if errors.Is(err, ErrUnsupportedPhoto) || errors.Is(err, ErrPhotoTooLarge) || errors.Is(err, ErrVideoTooLarge) {
			s.removeResumable(id)
		}
		return Media{}, err
	}
	if err := os.Remove(s.resumableInfoPath(id)); err != nil && !os.IsNotExist(err) {
		// The media is confirmed; reconciliation removes the leftover record later.
		slog.Warn("remove published upload record", "media_id", id, "error", err)
	}
	return media, nil
}

// readPending returns the declared metadata of pending upload id and the bytes
// already stored. A missing record or data file means no pending upload.
func (s *Store) readPending(id string) (handler.FileInfo, int64, error) {
	info, err := readResumableInfo(s.resumableInfoPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return handler.FileInfo{}, 0, ErrResumableNotFound
		}
		return handler.FileInfo{}, 0, err
	}
	stat, err := os.Stat(s.resumableBinPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return handler.FileInfo{}, 0, ErrResumableNotFound
		}
		return handler.FileInfo{}, 0, fmt.Errorf("inspect resumable upload: %w", err)
	}
	return info, stat.Size(), nil
}

func readResumableInfo(path string) (handler.FileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return handler.FileInfo{}, err
	}
	var info handler.FileInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return handler.FileInfo{}, fmt.Errorf("decode resumable upload record: %w", err)
	}
	return info, nil
}

// pendingResumableBytes sums, over every pending upload in tmp/tus, the declared
// bytes not yet on disk. Callers hold ledger.mu. A record without its data file
// (a leftover of a finished upload) reserves nothing.
func (s *Store) pendingResumableBytes() (uint64, error) {
	entries, err := os.ReadDir(s.paths.Tus)
	if err != nil {
		return 0, fmt.Errorf("list resumable uploads: %w", err)
	}
	var total uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".info") {
			continue
		}
		id := strings.TrimSuffix(name, ".info")
		info, err := readResumableInfo(filepath.Join(s.paths.Tus, name))
		if err != nil {
			slog.Warn("skip unreadable resumable upload record", "upload_id", id, "error", err)
			continue
		}
		stat, err := os.Stat(s.resumableBinPath(id))
		if err != nil {
			continue
		}
		if info.Size > stat.Size() {
			total += uint64(info.Size - stat.Size())
		}
	}
	return total, nil
}

// expireResumable removes pending uploads whose newest file is older than
// UploadResumeTTL. It skips an ID with a PATCH in flight, or whose ID lock is busy.
func (s *Store) expireResumable(ctx context.Context, report *ReconcileReport) error {
	if s.options.UploadResumeTTL <= 0 {
		return nil
	}
	entries, err := os.ReadDir(s.paths.Tus)
	if err != nil {
		return fmt.Errorf("list resumable uploads: %w", err)
	}
	seen := make(map[string]bool)
	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".info")
		if !ValidID(id) || seen[id] {
			continue
		}
		seen[id] = true
		errs = append(errs, s.expireResumableID(id, report))
	}
	return errors.Join(errs...)
}

func (s *Store) expireResumableID(id string, report *ReconcileReport) error {
	unlockID, ok := s.tryLockID(id)
	if !ok {
		report.SkippedActive++
		return nil
	}
	defer unlockID()
	// Holding patchMu while checking and removing keeps a PATCH from starting between the two.
	s.patchMu.Lock()
	defer s.patchMu.Unlock()
	if s.patching[id] > 0 {
		report.SkippedActive++
		return nil
	}
	var newest time.Time
	found := false
	for _, path := range []string{s.resumableBinPath(id), s.resumableInfoPath(id)} {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		found = true
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if !found || time.Since(newest) < s.options.UploadResumeTTL {
		return nil
	}
	s.removeResumableLocked(id)
	report.ExpiredResumable++
	return nil
}

// beginPatch records a PATCH in flight for id and returns the function that ends it.
func (s *Store) beginPatch(id string) func() {
	s.patchMu.Lock()
	defer s.patchMu.Unlock()
	s.patching[id]++
	return func() {
		s.patchMu.Lock()
		defer s.patchMu.Unlock()
		s.patching[id]--
		if s.patching[id] == 0 {
			delete(s.patching, id)
		}
	}
}

// removeResumable deletes the data and record of pending upload id.
func (s *Store) removeResumable(id string) {
	s.patchMu.Lock()
	defer s.patchMu.Unlock()
	s.removeResumableLocked(id)
}

// removeResumableLocked deletes both files of pending upload id. Callers hold patchMu.
func (s *Store) removeResumableLocked(id string) {
	for _, path := range []string{s.resumableBinPath(id), s.resumableInfoPath(id)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Warn("remove resumable upload file", "upload_id", id, "error", err)
		}
	}
}

func (s *Store) resumableBinPath(id string) string {
	return filepath.Join(s.paths.Tus, id)
}

func (s *Store) resumableInfoPath(id string) string {
	return filepath.Join(s.paths.Tus, id+".info")
}

// tusLogger keeps tusd's per-request info lines out of the server log; its
// warnings and errors still reach stderr. tusd takes an x/exp/slog logger.
func tusLogger() *expslog.Logger {
	return expslog.New(expslog.NewTextHandler(os.Stderr, &expslog.HandlerOptions{Level: expslog.LevelWarn}))
}

// jsonHTTPError is a tus error whose body is the application's JSON error format.
func jsonHTTPError(status int, code string) handler.Error {
	return handler.Error{
		ErrorCode: code,
		Message:   code,
		HTTPResponse: handler.HTTPResponse{
			StatusCode: status,
			Body:       "{\"error\":\"" + code + "\"}\n",
			Header:     handler.HTTPHeader{"Content-Type": "application/json; charset=utf-8"},
		},
	}
}
