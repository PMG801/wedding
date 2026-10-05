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
	return PhotoType{}, errors.New("unsupported photo signature")
}

type Options struct {
	MaxBytes        int64
	MaxConcurrent   int
	MinFreeBytes    uint64
}

type Media struct {
	ID, StorageName, MIME string
	SizeBytes, ConfirmedAt int64
	AlreadyConfirmed bool
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
	if !validID(id) {
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
		existing.AlreadyConfirmed = true
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Media{}, fmt.Errorf("check existing media: %w", err)
	}
	if declaredSize <= 0 || declaredSize > s.options.MaxBytes {
		return Media{}, fmt.Errorf("declared photo size must be between 1 and %d bytes", s.options.MaxBytes)
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
		return Media{}, errors.New("minimum free-disk threshold would be exceeded")
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
	photoType, err := DetectPhotoType(header)
	if err != nil {
		return Media{}, err
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
		return Media{}, errors.New("photo body does not match declared size")
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
	media := Media{ID: id, StorageName: storageName, MIME: photoType.MIME, SizeBytes: declaredSize, ConfirmedAt: now.UnixMilli()}
	_, err = s.db.ExecContext(ctx, "INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at) VALUES (?, ?, ?, ?, ?)", media.ID, media.StorageName, media.MIME, media.SizeBytes, media.ConfirmedAt)
	if err != nil {
		return Media{}, fmt.Errorf("confirm photo in database: %w", err)
	}
	return media, nil
}

func (s *Store) confirmed(ctx context.Context, id string) (Media, error) {
	var result Media
	err := s.db.QueryRowContext(ctx, "SELECT id, storage_name, media_type, size_bytes, confirmed_at FROM media WHERE id = ?", id).Scan(&result.ID, &result.StorageName, &result.MIME, &result.SizeBytes, &result.ConfirmedAt)
	return result, err
}
func (s *Store) lockID(ctx context.Context, id string) (func(), error) {
	idLocksMu.Lock()
	lock := idLocks[id]
	if lock == nil {
		lock = &idLock{token: make(chan struct{}, 1)}
		idLocks[id] = lock
	}
	lock.refs++
	idLocksMu.Unlock()
	select {
	case lock.token <- struct{}{}:
		return func() {
			<-lock.token
			idLocksMu.Lock()
			lock.refs--
			if lock.refs == 0 {
				delete(idLocks, id)
			}
			idLocksMu.Unlock()
		}, nil
	case <-ctx.Done():
		idLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(idLocks, id)
		}
		idLocksMu.Unlock()
		return nil, ctx.Err()
	}
}

func validID(id string) bool {
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
