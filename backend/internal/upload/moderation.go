package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// reconcileVisibility makes file locations follow SQLite: hidden rows keep
// their original in hidden/ and derivatives in tmp/*.staged; visible rows
// whose original is only in hidden/ get it back in originals/. Files in
// hidden/ without a row are left untouched.
func (s *Store) reconcileVisibility(ctx context.Context, report *ReconcileReport) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id, storage_name FROM media WHERE visibility = 'hidden'")
	if err != nil {
		return fmt.Errorf("list hidden media: %w", err)
	}
	var hidden []Media
	for rows.Next() {
		var media Media
		if err := rows.Scan(&media.ID, &media.StorageName); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read hidden media: %w", err)
		}
		hidden = append(hidden, media)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("read hidden media: %w", err)
	}
	var errs []error
	for _, media := range hidden {
		errs = append(errs, s.withIdleID(media.ID, report, func() error { return s.repairHidden(ctx, media, report) }))
	}
	entries, err := os.ReadDir(s.paths.Hidden)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("list hidden directory: %w", err))...)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		_, rest, found := strings.Cut(name, "_")
		if entry.IsDir() || !found || !ValidID(rest[:min(len(rest), 36)]) {
			continue
		}
		errs = append(errs, s.withIdleID(rest[:36], report, func() error { return s.repairShown(ctx, name, report) }))
	}
	return errors.Join(errs...)
}

// withIdleID runs repair under the ID lock, skipping IDs an upload holds.
func (s *Store) withIdleID(id string, report *ReconcileReport, repair func() error) error {
	unlock, ok := s.tryLockID(id)
	if !ok {
		report.SkippedActive++
		return nil
	}
	defer unlock()
	return repair()
}

func (s *Store) repairHidden(ctx context.Context, media Media, report *ReconcileReport) error {
	current, err := s.confirmed(ctx, media.ID)
	if err != nil || current.Visibility != "hidden" {
		return err
	}
	moves := [][2]string{{filepath.Join(s.paths.Originals, media.StorageName), filepath.Join(s.paths.Hidden, media.StorageName)}}
	for _, variant := range []string{"t", "d"} {
		moves = append(moves, [2]string{s.derivativePath(media.ID, variant), s.stagedDerivativePath(media.ID, variant)})
	}
	var errs []error
	for _, move := range moves {
		if _, err := os.Stat(move[0]); os.IsNotExist(err) {
			continue
		}
		if err := moveFile(move[0], move[1]); err != nil {
			errs = append(errs, fmt.Errorf("make %q private: %w", filepath.Base(move[0]), err))
			continue
		}
		report.VisibilityRepairs++
	}
	return errors.Join(errs...)
}

func (s *Store) repairShown(ctx context.Context, name string, report *ReconcileReport) error {
	var visibility string
	err := s.db.QueryRowContext(ctx, "SELECT visibility FROM media WHERE storage_name = ?", name).Scan(&visibility)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && visibility != "visible") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check hidden file %q in database: %w", name, err)
	}
	if err := moveFile(filepath.Join(s.paths.Hidden, name), filepath.Join(s.paths.Originals, name)); err != nil {
		return fmt.Errorf("return visible original %q: %w", name, err)
	}
	report.VisibilityRepairs++
	return nil
}
