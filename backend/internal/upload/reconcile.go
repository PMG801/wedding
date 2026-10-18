package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ReconcileReport counts what one reconciliation pass changed or skipped.
type ReconcileReport struct {
	RemovedPartials     int
	Quarantined         int
	PromotedDerivatives int
	RemovedStaged       int
	SkippedActive       int
}

// Reconcile repairs state left by interrupted work (ADR-0009): it removes
// abandoned partial files, publishes staged derivatives whose original is
// confirmed and visible, and moves originals without a database record to
// quarantine. It never deletes an original and skips any ID whose upload is
// in progress. A failure on one file does not stop the rest of the pass.
func (s *Store) Reconcile(ctx context.Context) (ReconcileReport, error) {
	var report ReconcileReport
	tmpErr := s.reconcileTmp(ctx, &report)
	originalsErr := s.quarantineUnindexedOriginals(ctx, &report)
	return report, errors.Join(tmpErr, originalsErr)
}

func (s *Store) reconcileTmp(ctx context.Context, report *ReconcileReport) error {
	entries, err := os.ReadDir(s.paths.Tmp)
	if err != nil {
		return fmt.Errorf("list tmp directory: %w", err)
	}
	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		switch {
		case strings.HasSuffix(name, ".part"):
			errs = append(errs, s.removeAbandonedPartial(name, report))
		case strings.HasSuffix(name, ".staged"):
			errs = append(errs, s.promoteStagedIfVisible(ctx, name, report))
		}
	}
	return errors.Join(errs...)
}

// removeAbandonedPartial deletes a .part file unless its upload holds the ID
// lock: partial files are only written while that lock is held.
func (s *Store) removeAbandonedPartial(name string, report *ReconcileReport) error {
	if id := name[:min(len(name), 36)]; ValidID(id) {
		unlock, ok := s.tryLockID(id)
		if !ok {
			report.SkippedActive++
			return nil
		}
		defer unlock()
	}
	if err := os.Remove(filepath.Join(s.paths.Tmp, name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove abandoned partial %q: %w", name, err)
	}
	report.RemovedPartials++
	return nil
}

func (s *Store) promoteStagedIfVisible(ctx context.Context, name string, report *ReconcileReport) error {
	base := strings.TrimSuffix(name, ".staged")
	id, variant, found := strings.Cut(base, "_")
	if !found || !ValidID(id) || (variant != "t" && variant != "d") {
		return nil
	}
	unlock, ok := s.tryLockID(id)
	if !ok {
		report.SkippedActive++
		return nil
	}
	defer unlock()
	original, err := s.confirmed(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.removeExpiredStaged(name, report)
	}
	if err == nil && original.Visibility != "visible" {
		// Hidden originals keep their derivatives private, ready to be shown again.
		return nil
	}
	if err != nil {
		return fmt.Errorf("check original of staged derivative %q: %w", name, err)
	}
	if err := s.promoteStagedDerivative(id, variant); err != nil {
		return fmt.Errorf("promote staged derivative %q: %w", name, err)
	}
	report.PromotedDerivatives++
	return nil
}

// removeExpiredStaged deletes a staged derivative whose original was never
// confirmed once it is older than StagedDerivativeTTL; derivatives need only a
// guest session, so without a TTL abandoned ones would accumulate in tmp.
// Callers hold the ID lock.
func (s *Store) removeExpiredStaged(name string, report *ReconcileReport) error {
	if s.options.StagedDerivativeTTL <= 0 {
		return nil
	}
	path := filepath.Join(s.paths.Tmp, name)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect staged derivative %q: %w", name, err)
	}
	if time.Since(info.ModTime()) < s.options.StagedDerivativeTTL {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove expired staged derivative %q: %w", name, err)
	}
	report.RemovedStaged++
	return nil
}

func (s *Store) quarantineUnindexedOriginals(ctx context.Context, report *ReconcileReport) error {
	entries, err := os.ReadDir(s.paths.Originals)
	if err != nil {
		return fmt.Errorf("list originals directory: %w", err)
	}
	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}
		errs = append(errs, s.quarantineIfUnindexed(ctx, entry.Name(), report))
	}
	return errors.Join(errs...)
}

func (s *Store) quarantineIfUnindexed(ctx context.Context, name string, report *ReconcileReport) error {
	// Storage names are "<timestamp>_<id><extension>". An upload being
	// confirmed holds its ID lock between the rename and the INSERT.
	if _, rest, found := strings.Cut(name, "_"); found {
		if id := rest[:min(len(rest), 36)]; ValidID(id) {
			unlock, ok := s.tryLockID(id)
			if !ok {
				report.SkippedActive++
				return nil
			}
			defer unlock()
		}
	}
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media WHERE storage_name = ?", name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check original %q in database: %w", name, err)
	}
	if exists > 0 {
		return nil
	}
	if err := moveWithoutReplace(filepath.Join(s.paths.Originals, name), s.paths.Quarantine, name); err != nil {
		return fmt.Errorf("quarantine unindexed original %q: %w", name, err)
	}
	report.Quarantined++
	return nil
}

// moveWithoutReplace moves a file into directory, choosing a numbered name
// when the original name is taken, so no quarantined file is overwritten.
func moveWithoutReplace(sourcePath, directory, name string) error {
	for attempt := 0; attempt < 1000; attempt++ {
		candidate := name
		if attempt > 0 {
			candidate = name + "." + strconv.Itoa(attempt)
		}
		destination := filepath.Join(directory, candidate)
		if err := os.Link(sourcePath, destination); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}
		if err := syncDirectory(directory); err != nil {
			return err
		}
		if err := os.Remove(sourcePath); err != nil {
			return fmt.Errorf("remove moved file: %w", err)
		}
		return syncDirectory(filepath.Dir(sourcePath))
	}
	return errors.New("no free quarantine file name")
}
