package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrMediaNotFound     = errors.New("media not found")
	ErrInvalidVisibility = errors.New("visibility must be hidden or visible")
)

// SetVisibility hides or shows one confirmed media item. It never deletes
// anything: hiding moves the original to hidden/ and its published derivatives
// back to private tmp/*.staged files, then records the state in SQLite;
// showing records the state first and then republishes the files. A file is
// therefore never served while SQLite says it is hidden. A failed step undoes
// what was already moved. Setting the current visibility changes nothing.
func (s *Store) SetVisibility(ctx context.Context, id, visibility string) (Media, error) {
	if visibility != "hidden" && visibility != "visible" {
		return Media{}, ErrInvalidVisibility
	}
	if !ValidID(id) {
		return Media{}, ErrMediaNotFound
	}
	unlock, err := s.lockID(ctx, id)
	if err != nil {
		return Media{}, err
	}
	defer unlock()

	media, err := s.confirmed(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrMediaNotFound
	}
	if err != nil {
		return Media{}, fmt.Errorf("read media: %w", err)
	}
	if media.Visibility == visibility {
		return media, nil
	}
	if visibility == "hidden" {
		err = s.hide(ctx, media)
	} else {
		err = s.show(ctx, media)
	}
	if err != nil {
		return Media{}, err
	}
	media.Visibility = visibility
	return media, nil
}

func (s *Store) hide(ctx context.Context, media Media) error {
	if err := moveFile(filepath.Join(s.paths.Originals, media.StorageName), filepath.Join(s.paths.Hidden, media.StorageName)); err != nil {
		return fmt.Errorf("move original to hidden: %w", err)
	}
	var moved []string
	rollback := func(cause error) error {
		for _, variant := range moved {
			if err := moveFile(s.stagedDerivativePath(media.ID, variant), s.derivativePath(media.ID, variant)); err != nil {
				cause = errors.Join(cause, fmt.Errorf("restore derivative %s: %w", variant, err))
			}
		}
		if err := moveFile(filepath.Join(s.paths.Hidden, media.StorageName), filepath.Join(s.paths.Originals, media.StorageName)); err != nil {
			cause = errors.Join(cause, fmt.Errorf("restore original: %w", err))
		}
		return cause
	}
	for _, variant := range []string{"t", "d"} {
		published := s.derivativePath(media.ID, variant)
		if _, err := os.Stat(published); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return rollback(fmt.Errorf("inspect derivative %s: %w", variant, err))
		}
		if err := moveFile(published, s.stagedDerivativePath(media.ID, variant)); err != nil {
			return rollback(fmt.Errorf("make derivative %s private: %w", variant, err))
		}
		moved = append(moved, variant)
	}
	if err := s.recordVisibility(ctx, media.ID, "hidden"); err != nil {
		return rollback(err)
	}
	return nil
}

func (s *Store) show(ctx context.Context, media Media) error {
	if err := s.recordVisibility(ctx, media.ID, "visible"); err != nil {
		return err
	}
	if err := moveFile(filepath.Join(s.paths.Hidden, media.StorageName), filepath.Join(s.paths.Originals, media.StorageName)); err != nil {
		cause := fmt.Errorf("move original out of hidden: %w", err)
		// Detached from ctx: a cancelled request must not leave SQLite
		// pointing at a file that is still in hidden/.
		if revertErr := s.recordVisibility(context.WithoutCancel(ctx), media.ID, "hidden"); revertErr != nil {
			cause = errors.Join(cause, revertErr)
		}
		return cause
	}
	// Failures are logged and retried by reconciliation.
	s.promotePendingDerivatives(media.ID)
	return nil
}

func (s *Store) recordVisibility(ctx context.Context, id, visibility string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE media SET visibility = ? WHERE id = ?", visibility, id); err != nil {
		return fmt.Errorf("record visibility %s: %w", visibility, err)
	}
	return nil
}

// moveFile moves source to destination without replacing a different file.
// It completes an interrupted earlier move: when source is gone and
// destination exists, or both are links to the same file, the move is done.
func moveFile(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			if _, statErr := os.Stat(destination); statErr == nil {
				return nil
			}
			return err
		case errors.Is(err, os.ErrExist):
			sourceInfo, sourceErr := os.Stat(source)
			destinationInfo, destinationErr := os.Stat(destination)
			if sourceErr != nil || destinationErr != nil || !os.SameFile(sourceInfo, destinationInfo) {
				return fmt.Errorf("destination %q already holds a different file", destination)
			}
		default:
			return err
		}
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return fmt.Errorf("remove moved file: %w", err)
	}
	return syncDirectory(filepath.Dir(source))
}
