// Package upload implements bounded, atomic persistence for original photos.
package upload

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PMG801/wedding/internal/storage"
)

var (
	ErrUnsupportedPhoto         = errors.New("unsupported photo signature")
	ErrPhotoSizeMismatch        = errors.New("photo body does not match declared size")
	ErrInsufficientStorage      = errors.New("minimum free-disk threshold would be exceeded")
	ErrTooManyStagedDerivatives = errors.New("too many staged derivatives of unconfirmed originals")
	ErrMediaHidden              = errors.New("media is hidden")
	ErrPhotoTooLarge            = errors.New("photo exceeds the configured size limit")
	ErrVideoTooLarge            = errors.New("video exceeds the configured size limit")
	ErrDerivativeTooLarge       = errors.New("derivative exceeds the configured size limit")
)

type PhotoType struct{ MIME, Extension string }

// DetectPhotoType recognizes JPEG, PNG, HEIC and HEIF signatures.
func DetectPhotoType(header []byte) (PhotoType, error) {
	if bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}) {
		return PhotoType{"image/jpeg", ".jpg"}, nil
	}
	if bytes.HasPrefix(header, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return PhotoType{"image/png", ".png"}, nil
	}
	if len(header) >= 12 && string(header[4:8]) == "ftyp" {
		brands := string(header[8:])
		for _, brand := range []string{"heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs"} {
			if strings.Contains(brands, brand) {
				return PhotoType{"image/heic", ".heic"}, nil
			}
		}
		for _, brand := range []string{"heif", "mif1", "msf1"} {
			if strings.Contains(brands, brand) {
				return PhotoType{"image/heif", ".heif"}, nil
			}
		}
	}
	return PhotoType{}, ErrUnsupportedPhoto
}

type Options struct {
	// MaxBytes limits photos and derivatives.
	MaxBytes int64
	// MaxVideoBytes limits videos; zero means the same limit as MaxBytes.
	MaxVideoBytes int64
	MaxConcurrent int
	MinFreeBytes  uint64
	// MaxDerivativeBytes limits client-generated derivatives; zero means MaxBytes.
	MaxDerivativeBytes int64
	// StagedDerivativeTTL is how long reconciliation keeps a staged derivative
	// whose original was never confirmed; zero keeps it forever.
	StagedDerivativeTTL time.Duration
	// UploadResumeTTL expires resumable uploads idle for longer; zero keeps them.
	UploadResumeTTL time.Duration
	// MaxPendingUploads caps the resumable uploads pending in tmp/tus, and twice
	// that many staged derivatives of unconfirmed originals; zero means no cap.
	MaxPendingUploads int
	// EmptyUploadTTL expires a pending upload that has received no bytes sooner
	// than UploadResumeTTL (the shorter of the two applies); zero disables it.
	EmptyUploadTTL time.Duration
	// FreeSpace overrides the free-space probe; tests use it to simulate a full disk.
	FreeSpace func(path string) (uint64, error)
}

var videoBrands = map[string]PhotoType{
	"isom": {"video/mp4", ".mp4"}, "iso2": {"video/mp4", ".mp4"}, "mp41": {"video/mp4", ".mp4"},
	"mp42": {"video/mp4", ".mp4"}, "avc1": {"video/mp4", ".mp4"}, "M4V ": {"video/mp4", ".mp4"},
	"qt  ": {"video/quicktime", ".mov"},
}

// DetectMediaType recognizes the supported photo signatures and MP4/MOV videos.
// Videos are identified by the major brand of their ISO base media ftyp box.
func DetectMediaType(header []byte) (PhotoType, error) {
	if len(header) >= 12 && string(header[4:8]) == "ftyp" {
		if video, ok := videoBrands[string(header[8:12])]; ok {
			return video, nil
		}
	}
	return DetectPhotoType(header)
}

func (s *Store) maxVideoBytes() int64 {
	if s.options.MaxVideoBytes > 0 {
		return s.options.MaxVideoBytes
	}
	return s.options.MaxBytes
}

func (s *Store) maxMediaBytes() int64 {
	if limit := s.maxVideoBytes(); limit > s.options.MaxBytes {
		return limit
	}
	return s.options.MaxBytes
}

type Media struct {
	ID, StorageName, MIME, Visibility string
	SizeBytes, ConfirmedAt            int64
	AlreadyConfirmed                  bool
}
type idLock struct {
	token chan struct{}
	refs  int
}

var idLocksMu sync.Mutex
var idLocks = make(map[string]*idLock)

type Store struct {
	db        *sql.DB
	paths     storage.Paths
	options   Options
	semaphore chan struct{}
	ledger    diskLedger
	// freeSpace overrides statfs in tests.
	freeSpace func(path string) (uint64, error)
	// patchMu guards patching, the resumable PATCH requests in flight per ID.
	patchMu  sync.Mutex
	patching map[string]int
}

// NewStore constructs a photo store using already-initialized paths and SQLite.
func NewStore(database *sql.DB, paths storage.Paths, options Options) (*Store, error) {
	if database == nil || paths.Tmp == "" || paths.Originals == "" {
		return nil, errors.New("database and initialized storage paths are required")
	}
	if options.MaxBytes <= 0 || options.MaxConcurrent <= 0 {
		return nil, errors.New("positive photo size and concurrency limits are required")
	}
	return &Store{db: database, paths: paths, options: options, semaphore: make(chan struct{}, options.MaxConcurrent), freeSpace: options.FreeSpace, patching: make(map[string]int)}, nil
}

// checkMediaType detects the type from the leading bytes and applies the size
// limit of that type.
func (s *Store) checkMediaType(header []byte, size int64) (PhotoType, error) {
	photoType, err := DetectMediaType(header)
	if err != nil {
		return PhotoType{}, err
	}
	if strings.HasPrefix(photoType.MIME, "video/") {
		if size > s.maxVideoBytes() {
			return PhotoType{}, ErrVideoTooLarge
		}
	} else if size > s.options.MaxBytes {
		return PhotoType{}, ErrPhotoTooLarge
	}
	return photoType, nil
}

// publishPart is the publication tail of resumable uploads (ADR-0014). It checks
// the complete file at partPath, syncs it, moves it into originals under its
// chronological name, syncs the directory and confirms it in SQLite. The caller holds the ID lock. A failure before the move
// leaves partPath in place. A failure after it moves the file back, so the
// complete upload survives for a retry. capturedAt is the raw captured_at
// metadata (ADR-0015): stored only when NormalizeCapturedAt accepts it, else NULL.
func (s *Store) publishPart(ctx context.Context, id, partPath string, declaredSize int64, capturedAt string) (Media, error) {
	photoType, err := s.checkPart(partPath, declaredSize)
	if err != nil {
		return Media{}, err
	}

	now := time.Now().UTC()
	storageName := now.Format("20060102T150405.000000000Z") + "_" + id + photoType.Extension
	finalPath := filepath.Join(s.paths.Originals, storageName)
	if err := os.Rename(partPath, finalPath); err != nil {
		return Media{}, fmt.Errorf("publish photo: %w", err)
	}
	restore := func() { _ = os.Rename(finalPath, partPath) }
	if err := syncDirectory(s.paths.Originals); err != nil {
		restore()
		return Media{}, fmt.Errorf("sync originals directory: %w", err)
	}
	media := Media{ID: id, StorageName: storageName, MIME: photoType.MIME, Visibility: "visible", SizeBytes: declaredSize, ConfirmedAt: now.UnixMilli()}
	var captured sql.NullString
	if value, ok := NormalizeCapturedAt(capturedAt); ok {
		captured = sql.NullString{String: value, Valid: true}
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, captured_at) VALUES (?, ?, ?, ?, ?, ?)", media.ID, media.StorageName, media.MIME, media.SizeBytes, media.ConfirmedAt, captured); err != nil {
		restore()
		return Media{}, fmt.Errorf("confirm photo in database: %w", err)
	}
	s.promotePendingDerivatives(id)
	return media, nil
}

// checkPart verifies the size, the signature and the per-type limit of the
// complete file at partPath, and syncs it to disk.
func (s *Store) checkPart(partPath string, declaredSize int64) (PhotoType, error) {
	file, err := os.Open(partPath)
	if err != nil {
		return PhotoType{}, fmt.Errorf("open complete upload: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return PhotoType{}, fmt.Errorf("inspect complete upload: %w", err)
	}
	if info.Size() != declaredSize {
		return PhotoType{}, ErrPhotoSizeMismatch
	}
	header := make([]byte, int(min(declaredSize, 64)))
	if _, err := io.ReadFull(file, header); err != nil {
		return PhotoType{}, fmt.Errorf("read photo signature: %w", err)
	}
	photoType, err := s.checkMediaType(header, declaredSize)
	if err != nil {
		return PhotoType{}, err
	}
	if err := file.Sync(); err != nil {
		return PhotoType{}, fmt.Errorf("sync photo: %w", err)
	}
	return photoType, nil
}

func (s *Store) confirmed(ctx context.Context, id string) (Media, error) {
	var result Media
	err := s.db.QueryRowContext(ctx, "SELECT id, storage_name, media_type, size_bytes, confirmed_at, visibility FROM media WHERE id = ?", id).Scan(&result.ID, &result.StorageName, &result.MIME, &result.SizeBytes, &result.ConfirmedAt, &result.Visibility)
	return result, err
}
func (s *Store) lockID(ctx context.Context, id string) (func(), error) {
	lock := acquireIDRef(id)
	select {
	case lock.token <- struct{}{}:
		return func() {
			<-lock.token
			releaseIDRef(id, lock)
		}, nil
	case <-ctx.Done():
		releaseIDRef(id, lock)
		return nil, ctx.Err()
	}
}

// tryLockID takes the per-ID lock only when no upload currently holds it.
func (s *Store) tryLockID(id string) (func(), bool) {
	lock := acquireIDRef(id)
	select {
	case lock.token <- struct{}{}:
		return func() {
			<-lock.token
			releaseIDRef(id, lock)
		}, true
	default:
		releaseIDRef(id, lock)
		return nil, false
	}
}

func acquireIDRef(id string) *idLock {
	idLocksMu.Lock()
	defer idLocksMu.Unlock()
	lock := idLocks[id]
	if lock == nil {
		lock = &idLock{token: make(chan struct{}, 1)}
		idLocks[id] = lock
	}
	lock.refs++
	return lock
}

func releaseIDRef(id string, lock *idLock) {
	idLocksMu.Lock()
	defer idLocksMu.Unlock()
	lock.refs--
	if lock.refs == 0 {
		delete(idLocks, id)
	}
}

// ValidID reports whether id is a canonical lowercase UUIDv4.
func ValidID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || id[14] != '4' {
		return false
	}
	if id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b' {
		return false
	}
	for i, char := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
