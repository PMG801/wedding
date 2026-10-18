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

// DiskUsage measures the data volume and the in-flight reservations.
func (s *Store) DiskUsage() (DiskUsage, error) {
	free, err := s.free()
	if err != nil {
		return DiskUsage{}, err
	}
	return DiskUsage{FreeBytes: free, ReservedBytes: uint64(max(s.ledger.reserved.Load(), 0)), MinFreeBytes: s.options.MinFreeBytes}, nil
}

// reserve admits declaredSize bytes only if the free space minus every
// outstanding reservation still leaves MinFreeBytes after writing them.
func (s *Store) reserve(declaredSize int64) (*reservation, error) {
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	// Read the reservations before statfs: bytes written in between then count
	// twice (conservative) instead of not at all.
	reserved := uint64(max(s.ledger.reserved.Load(), 0))
	free, err := s.free()
	if err != nil {
		return nil, err
	}
	need := reserved + uint64(declaredSize)
	if free < s.options.MinFreeBytes || need > free-s.options.MinFreeBytes {
		return nil, ErrInsufficientStorage
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
