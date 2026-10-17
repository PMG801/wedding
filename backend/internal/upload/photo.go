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
	"syscall"
	"time"

	"github.com/PMG801/wedding/internal/storage"
)

var (
	ErrUnsupportedPhoto    = errors.New("unsupported photo signature")
	ErrPhotoSizeMismatch   = errors.New("photo body does not match declared size")
	ErrInsufficientStorage = errors.New("minimum free-disk threshold would be exceeded")
	ErrMediaHidden         = errors.New("media is hidden")
	ErrMediaTooLarge       = errors.New("media exceeds the largest configured size limit")
	ErrPhotoTooLarge       = errors.New("photo exceeds the configured size limit")
	ErrVideoTooLarge       = errors.New("video exceeds the configured size limit")
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
}

// NewStore constructs a photo store using already-initialized paths and SQLite.
func NewStore(database *sql.DB, paths storage.Paths, options Options) (*Store, error) {
	if database == nil || paths.Tmp == "" || paths.Originals == "" {
		return nil, errors.New("database and initialized storage paths are required")
	}
	if options.MaxBytes <= 0 || options.MaxConcurrent <= 0 {
		return nil, errors.New("positive photo size and concurrency limits are required")
	}
	return &Store{db: database, paths: paths, options: options, semaphore: make(chan struct{}, options.MaxConcurrent)}, nil
}

// SavePhoto validates and streams a declared-size photo into the originals directory.
func (s *Store) SavePhoto(ctx context.Context, id string, declaredSize int64, body io.Reader) (Media, error) {
	if !ValidID(id) {
		return Media{}, errors.New("media ID must be a canonical lowercase UUIDv4")
	}
	if body == nil {
		return Media{}, errors.New("photo body is required")
	}
	unlock, err := s.lockID(ctx, id)
	if err != nil {
		return Media{}, err
	}
	defer unlock()

	if existing, err := s.confirmed(ctx, id); err == nil {
		if existing.Visibility == "visible" {
			s.promotePendingDerivatives(id)
		}
		existing.AlreadyConfirmed = true
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Media{}, fmt.Errorf("check existing media: %w", err)
	}
	if declaredSize <= 0 || declaredSize > s.maxMediaBytes() {
		if declaredSize > s.maxMediaBytes() {
			return Media{}, ErrMediaTooLarge
		}
		return Media{}, errors.New("declared media size must be positive")
	}
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return Media{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return Media{}, err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.paths.Tmp, &stat); err != nil {
		return Media{}, fmt.Errorf("check available disk space: %w", err)
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if free < s.options.MinFreeBytes || uint64(declaredSize) > free-s.options.MinFreeBytes {
		return Media{}, ErrInsufficientStorage
	}

	partPath := filepath.Join(s.paths.Tmp, id+".part")
	if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
		return Media{}, fmt.Errorf("remove stale partial upload: %w", err)
	}
	file, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Media{}, fmt.Errorf("create partial upload: %w", err)
	}
	partExists := true
	defer func() {
		_ = file.Close()
		if partExists {
			_ = os.Remove(partPath)
		}
	}()

	headerSize := declaredSize
	if headerSize > 64 {
		headerSize = 64
	}
	header := make([]byte, int(headerSize))
	if _, err := io.ReadFull(contextReader{ctx: ctx, reader: body}, header); err != nil {
		return Media{}, fmt.Errorf("read photo signature: %w", err)
	}
	photoType, err := DetectMediaType(header)
	if err != nil {
		return Media{}, err
	}
	if strings.HasPrefix(photoType.MIME, "video/") {
		if declaredSize > s.maxVideoBytes() {
			return Media{}, ErrVideoTooLarge
		}
	} else if declaredSize > s.options.MaxBytes {
		return Media{}, ErrPhotoTooLarge
	}
	if _, err := file.Write(header); err != nil {
		return Media{}, fmt.Errorf("write photo: %w", err)
	}
	remaining := declaredSize - int64(len(header))
	if _, err := io.CopyN(file, contextReader{ctx: ctx, reader: body}, remaining); err != nil {
		return Media{}, fmt.Errorf("stream photo: %w", err)
	}
	var extra [1]byte
	if n, err := (contextReader{ctx: ctx, reader: body}).Read(extra[:]); n != 0 || err != io.EOF {
		if err != nil && err != io.EOF {
			return Media{}, fmt.Errorf("check photo size: %w", err)
		}
		return Media{}, ErrPhotoSizeMismatch
	}
	if err := ctx.Err(); err != nil {
		return Media{}, err
	}
	if err := file.Sync(); err != nil {
		return Media{}, fmt.Errorf("sync photo: %w", err)
	}
	if err := file.Close(); err != nil {
		return Media{}, fmt.Errorf("close photo: %w", err)
	}

	now := time.Now().UTC()
	storageName := now.Format("20060102T150405.000000000Z") + "_" + id + photoType.Extension
	finalPath := filepath.Join(s.paths.Originals, storageName)
	if err := os.Rename(partPath, finalPath); err != nil {
		return Media{}, fmt.Errorf("publish photo: %w", err)
	}
	partExists = false
	dir, err := os.Open(s.paths.Originals)
	if err != nil {
		return Media{}, fmt.Errorf("open originals directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return Media{}, fmt.Errorf("sync originals directory: %w", err)
	}
	_ = dir.Close()
	media := Media{ID: id, StorageName: storageName, MIME: photoType.MIME, Visibility: "visible", SizeBytes: declaredSize, ConfirmedAt: now.UnixMilli()}
	_, err = s.db.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES (?, ?, ?, ?, ?)", media.ID, media.StorageName, media.MIME, media.SizeBytes, media.ConfirmedAt)
	if err != nil {
		return Media{}, fmt.Errorf("confirm photo in database: %w", err)
	}
	s.promotePendingDerivatives(id)
	return media, nil
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
