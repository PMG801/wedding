package upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

const (
	secondID = "223e4567-e89b-42d3-a456-426614174001"
	thirdID  = "323e4567-e89b-42d3-a456-426614174002"
)

func pngOfSize(size int) []byte {
	body := make([]byte, size)
	copy(body, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	return body
}

// splitReader serves the first part of its data, then blocks until released
// so a test can observe an upload that is still in progress.
type splitReader struct {
	data       []byte
	split      int
	served     int
	blocked    chan struct{}
	release    chan struct{}
	blockedSet bool
}

func newSplitReader(data []byte, split int) *splitReader {
	return &splitReader{data: data, split: split, blocked: make(chan struct{}), release: make(chan struct{})}
}

func (r *splitReader) Read(p []byte) (int, error) {
	if r.served >= len(r.data) {
		return 0, io.EOF
	}
	if r.served == r.split && !r.blockedSet {
		r.blockedSet = true
		close(r.blocked)
		<-r.release
	}
	limit := len(r.data)
	if r.served < r.split {
		limit = r.split
	}
	n := copy(p, r.data[r.served:limit])
	r.served += n
	return n, nil
}

func newReservationStore(t *testing.T, free uint64) *Store {
	t.Helper()
	store, _ := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 4, MinFreeBytes: 100})
	store.freeSpace = func(string) (uint64, error) { return free, nil }
	return store
}

func startHeldPhoto(t *testing.T, store *Store, id string, body []byte, split int) (*splitReader, chan error) {
	t.Helper()
	reader := newSplitReader(body, split)
	done := make(chan error, 1)
	go func() {
		_, err := store.SavePhoto(context.Background(), id, int64(len(body)), reader)
		done <- err
	}()
	select {
	case <-reader.blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("held upload did not reach its blocking point")
	}
	return reader, done
}

func TestConcurrentUploadsCannotOvershootFreeDiskThreshold(t *testing.T) {
	// 250 free, 100 minimum: room for exactly one 100-byte upload at a time.
	store := newReservationStore(t, 250)
	first, firstDone := startHeldPhoto(t, store, testID, pngOfSize(100), 8)

	if _, err := store.SavePhoto(context.Background(), secondID, 100, bytes.NewReader(pngOfSize(100))); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("second concurrent photo error = %v; want ErrInsufficientStorage", err)
	}
	if _, err := store.SaveDerivative(context.Background(), thirdID, "t", 100, bytes.NewReader(make([]byte, 100))); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("concurrent derivative error = %v; want ErrInsufficientStorage", err)
	}

	close(first.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first photo = %v", err)
	}
	if usage, err := store.DiskUsage(); err != nil || usage.ReservedBytes != 0 {
		t.Fatalf("DiskUsage() after completion = %+v, %v; want no reservation", usage, err)
	}
	if _, err := store.SavePhoto(context.Background(), secondID, 100, bytes.NewReader(pngOfSize(100))); err != nil {
		t.Fatalf("photo after the reservation was released = %v", err)
	}
}

func TestDiskUsageReportsOnlyBytesNotYetWritten(t *testing.T) {
	store := newReservationStore(t, 10_000)
	// The 64-byte signature and 76 more bytes are written; 60 remain.
	held, done := startHeldPhoto(t, store, testID, pngOfSize(200), 140)
	usage, err := store.DiskUsage()
	if err != nil || usage.FreeBytes != 10_000 || usage.MinFreeBytes != 100 || usage.ReservedBytes != 60 {
		t.Fatalf("DiskUsage() during upload = %+v, %v; want free 10000, min 100, reserved 60", usage, err)
	}
	close(held.release)
	if err := <-done; err != nil {
		t.Fatalf("held photo = %v", err)
	}
}

func TestFailedUploadReleasesItsReservation(t *testing.T) {
	store := newReservationStore(t, 250)
	// Declares 100 bytes but sends 50: the upload fails as a size mismatch.
	if _, err := store.SavePhoto(context.Background(), testID, 100, bytes.NewReader(pngOfSize(50))); err == nil {
		t.Fatal("short body was accepted")
	}
	if usage, err := store.DiskUsage(); err != nil || usage.ReservedBytes != 0 {
		t.Fatalf("DiskUsage() after failure = %+v, %v; want no reservation", usage, err)
	}
	if _, err := store.SavePhoto(context.Background(), secondID, 100, bytes.NewReader(pngOfSize(100))); err != nil {
		t.Fatalf("photo after a failed upload = %v", err)
	}
	if entries, err := os.ReadDir(store.paths.Tmp); err != nil || len(entries) != 0 {
		t.Fatalf("tmp entries = %v, %v; want none", entries, err)
	}
}
