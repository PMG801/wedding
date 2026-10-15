package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

var ErrInvalidDerivativeVariant = errors.New("derivative variant must be t or d")

type Derivative struct {
	ID, Variant      string
	SizeBytes        int64
	AlreadyConfirmed bool
}

// SaveDerivative streams one client-generated JPEG derivative. Derivatives
// uploaded before their original remain private in tmp until that original is
// confirmed in SQLite.
func (s *Store) SaveDerivative(ctx context.Context, id, variant string, declaredSize int64, body io.Reader) (Derivative, error) {
	if !ValidID(id) {
		return Derivative{}, errors.New("media ID must be a canonical lowercase UUIDv4")
	}
	if variant != "t" && variant != "d" {
		return Derivative{}, ErrInvalidDerivativeVariant
	}
	if body == nil {
		return Derivative{}, errors.New("derivative body is required")
	}

	unlock, err := s.lockID(ctx, id)
	if err != nil {
		return Derivative{}, err
	}
	defer unlock()

	original, confirmedErr := s.confirmed(ctx, id)
	confirmed := confirmedErr == nil
	if confirmedErr != nil && !errors.Is(confirmedErr, sql.ErrNoRows) {
		return Derivative{}, fmt.Errorf("check existing media: %w", confirmedErr)
	}
	if confirmed && original.Visibility != "visible" {
		return Derivative{}, ErrMediaHidden
	}
	if confirmed {
		targetPath := s.derivativePath(id, variant)
		if info, err := os.Stat(targetPath); err == nil {
			return Derivative{ID: id, Variant: variant, SizeBytes: info.Size(), AlreadyConfirmed: true}, nil
		} else if !os.IsNotExist(err) {
			return Derivative{}, fmt.Errorf("inspect published derivative: %w", err)
		}
	}

	stagedPath := s.stagedDerivativePath(id, variant)
	if info, err := os.Stat(stagedPath); err == nil {
		if confirmed {
			if err := s.promoteStagedDerivative(id, variant); err != nil {
				return Derivative{}, fmt.Errorf("publish staged derivative: %w", err)
			}
		}
		return Derivative{ID: id, Variant: variant, SizeBytes: info.Size(), AlreadyConfirmed: true}, nil
	} else if !os.IsNotExist(err) {
		return Derivative{}, fmt.Errorf("inspect staged derivative: %w", err)
	}

	if declaredSize <= 0 || declaredSize > s.options.MaxBytes {
		return Derivative{}, fmt.Errorf("declared derivative size must be between 1 and %d bytes", s.options.MaxBytes)
	}
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return Derivative{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return Derivative{}, err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.paths.Tmp, &stat); err != nil {
		return Derivative{}, fmt.Errorf("check available disk space: %w", err)
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if free < s.options.MinFreeBytes || uint64(declaredSize) > free-s.options.MinFreeBytes {
		return Derivative{}, ErrInsufficientStorage
	}

	partPath := s.derivativePartPath(id, variant)
	file, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Derivative{}, fmt.Errorf("create partial derivative: %w", err)
	}
	partExists := true
	defer func() {
		_ = file.Close()
		if partExists {
			_ = os.Remove(partPath)
		}
	}()

	headerSize := declaredSize
	if headerSize > 3 {
		headerSize = 3
	}
	header := make([]byte, int(headerSize))
	if _, err := io.ReadFull(contextReader{ctx: ctx, reader: body}, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return Derivative{}, ErrPhotoSizeMismatch
		}
		return Derivative{}, fmt.Errorf("read derivative signature: %w", err)
	}
	photoType, err := DetectPhotoType(header)
	if err != nil || photoType.MIME != "image/jpeg" {
		return Derivative{}, ErrUnsupportedPhoto
	}
	if _, err := file.Write(header); err != nil {
		return Derivative{}, fmt.Errorf("write derivative: %w", err)
	}
	remaining := declaredSize - int64(len(header))
	if remaining > 0 {
		if _, err := io.CopyN(file, contextReader{ctx: ctx, reader: body}, remaining); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return Derivative{}, ErrPhotoSizeMismatch
			}
			return Derivative{}, fmt.Errorf("stream derivative: %w", err)
		}
	}
	var extra [1]byte
	if n, err := (contextReader{ctx: ctx, reader: body}).Read(extra[:]); n != 0 || err != io.EOF {
		if err != nil && err != io.EOF {
			return Derivative{}, fmt.Errorf("check derivative size: %w", err)
		}
		return Derivative{}, ErrPhotoSizeMismatch
	}
	if err := ctx.Err(); err != nil {
		return Derivative{}, err
	}
	if err := file.Sync(); err != nil {
		return Derivative{}, fmt.Errorf("sync derivative: %w", err)
	}
	if err := file.Close(); err != nil {
		return Derivative{}, fmt.Errorf("close derivative: %w", err)
	}

	destination := stagedPath
	if confirmed {
		destination = s.derivativePath(id, variant)
	}
	alreadySaved, err := publishWithoutReplace(partPath, destination)
	if err != nil {
		return Derivative{}, fmt.Errorf("publish derivative: %w", err)
	}
	partExists = false
	return Derivative{ID: id, Variant: variant, SizeBytes: declaredSize, AlreadyConfirmed: alreadySaved}, nil
}

func (s *Store) derivativePartPath(id, variant string) string {
	return filepath.Join(s.paths.Tmp, id+"_"+variant+".part")
}

func (s *Store) stagedDerivativePath(id, variant string) string {
	return filepath.Join(s.paths.Tmp, id+"_"+variant+".staged")
}

func (s *Store) derivativePath(id, variant string) string {
	return filepath.Join(s.paths.Derived, DerivativeFileName(id, variant))
}

// DerivativeFileName is the published file name of one JPEG derivative variant.
func DerivativeFileName(id, variant string) string {
	return id + "_" + variant + ".jpg"
}

func (s *Store) promotePendingDerivatives(id string) {
	for _, variant := range []string{"t", "d"} {
		if err := s.promoteStagedDerivative(id, variant); err != nil {
			slog.Error("publish staged photo derivative", "media_id", id, "variant", variant, "error", err)
		}
	}
}

func (s *Store) promoteStagedDerivative(id, variant string) error {
	stagedPath := s.stagedDerivativePath(id, variant)
	if _, err := os.Stat(stagedPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect staged derivative: %w", err)
	}
	if _, err := os.Stat(s.derivativePath(id, variant)); err == nil {
		if err := os.Remove(stagedPath); err != nil {
			return fmt.Errorf("remove redundant staged derivative: %w", err)
		}
		return syncDirectory(s.paths.Tmp)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect published derivative: %w", err)
	}
	if _, err := publishWithoutReplace(stagedPath, s.derivativePath(id, variant)); err != nil {
		return fmt.Errorf("move staged derivative into published storage: %w", err)
	}
	return nil
}

// publishWithoutReplace atomically links a complete temporary file at its
// destination, refusing to overwrite a previously accepted derivative.
func publishWithoutReplace(sourcePath, destinationPath string) (bool, error) {
	if err := os.Link(sourcePath, destinationPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			if removeErr := os.Remove(sourcePath); removeErr != nil {
				return false, fmt.Errorf("remove duplicate temporary file: %w", removeErr)
			}
			return true, nil
		}
		return false, err
	}
	if err := os.Remove(sourcePath); err != nil {
		return false, fmt.Errorf("remove linked temporary file: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destinationPath)); err != nil {
		return false, err
	}
	if filepath.Dir(sourcePath) != filepath.Dir(destinationPath) {
		if err := syncDirectory(filepath.Dir(sourcePath)); err != nil {
			return false, err
		}
	}
	return false, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
