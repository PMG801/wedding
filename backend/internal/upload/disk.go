package upload

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"syscall"
)

// DiskUsage reports the free space of the data volume, the bytes reserved by
// uploads still in progress, and the configured minimum free threshold.
type DiskUsage struct {
	FreeBytes, ReservedBytes, MinFreeBytes uint64
}

// diskLedger tracks the bytes that accepted uploads have declared but not yet
// written. statfs only sees bytes already on disk, so without the ledger
// concurrent uploads could all pass the free-space check against the same
// value and together fill the volume.
type diskLedger struct {
	mu       sync.Mutex // serializes check-and-reserve
	reserved atomic.Int64
}

type reservation struct {
	ledger    *diskLedger
	remaining int64
}

func statfsFree(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("check available disk space: %w", err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func (s *Store) free() (uint64, error) {
	if s.freeSpace != nil {
		return s.freeSpace(s.paths.Tmp)
	}
	return statfsFree(s.paths.Tmp)
}

// DiskUsage measures the data volume and the in-flight reservations: the
// derivative ledger plus the resumable uploads still waiting for bytes.
func (s *Store) DiskUsage() (DiskUsage, error) {
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	// Reservations first, statfs last, as in admitLocked.
	reserved, err := s.reservedLocked()
	if err != nil {
		return DiskUsage{}, err
	}
	free, err := s.free()
	if err != nil {
		return DiskUsage{}, err
	}
	return DiskUsage{FreeBytes: free, ReservedBytes: reserved, MinFreeBytes: s.options.MinFreeBytes}, nil
}

// reservedLocked returns the bytes promised to uploads that have not reached
// the disk yet. Callers hold ledger.mu.
func (s *Store) reservedLocked() (uint64, error) {
	pending, err := s.pendingResumableBytes()
	if err != nil {
		return 0, err
	}
	return pending + uint64(max(s.ledger.reserved.Load(), 0)), nil
}

// admitLocked fails with ErrInsufficientStorage unless declaredSize more bytes
// still leave MinFreeBytes free after every outstanding reservation. Callers hold
// ledger.mu, so no concurrent admission can pass against the same free space.
func (s *Store) admitLocked(declaredSize int64) error {
	// Read the reservations before statfs: bytes written in between then count
	// twice (conservative) instead of not at all.
	reserved, err := s.reservedLocked()
	if err != nil {
		return err
	}
	free, err := s.free()
	if err != nil {
		return err
	}
	need := reserved + uint64(declaredSize)
	if free < s.options.MinFreeBytes || need > free-s.options.MinFreeBytes {
		return ErrInsufficientStorage
	}
	return nil
}

// reserve admits declaredSize bytes for a photo or derivative and records them in the ledger.
func (s *Store) reserve(declaredSize int64) (*reservation, error) {
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	if err := s.admitLocked(declaredSize); err != nil {
		return nil, err
	}
	s.ledger.reserved.Add(declaredSize)
	return &reservation{ledger: &s.ledger, remaining: declaredSize}, nil
}

// wrote moves n bytes from the reservation to the disk, where statfs sees them.
func (r *reservation) wrote(n int) {
	used := min(int64(n), r.remaining)
	r.remaining -= used
	r.ledger.reserved.Add(-used)
}

// release returns whatever was not written; call it once the upload ends.
func (r *reservation) release() {
	r.ledger.reserved.Add(-r.remaining)
	r.remaining = 0
}

// writer counts bytes written through it against the reservation.
func (r *reservation) writer(w io.Writer) io.Writer { return reservedWriter{w: w, r: r} }

type reservedWriter struct {
	w io.Writer
	r *reservation
}

func (w reservedWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.r.wrote(n)
	return n, err
}
