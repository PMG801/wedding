package upload

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
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

func newReservationStore(t *testing.T, free uint64) *Store {
	t.Helper()
	store, _ := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 4, MinFreeBytes: 100})
	store.freeSpace = func(string) (uint64, error) { return free, nil }
	return store
}

// A declared upload reserves its whole size from creation until its bytes reach
// the disk, so a second creation that would overshoot the free-space threshold is refused.
func TestPendingUploadsCannotOvershootFreeDiskThreshold(t *testing.T) {
	// 250 free, 100 minimum: room for exactly one 100-byte upload at a time.
	store := newReservationStore(t, 250)
	resumable := tusSession(t, store)
	if created := tusCreateUpload(resumable, testID, 100); created.Code != http.StatusCreated {
		t.Fatalf("first creation = %d %q; want 201", created.Code, created.Body.String())
	}

	if created := tusCreateUpload(resumable, secondID, 100); created.Code != http.StatusInsufficientStorage {
		t.Fatalf("second creation while the first is pending = %d; want 507", created.Code)
	}
	if _, err := store.SaveDerivative(context.Background(), thirdID, "t", 100, bytes.NewReader(make([]byte, 100))); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("derivative while the upload is pending error = %v; want ErrInsufficientStorage", err)
	}

	if patched := tusPatchUpload(resumable, testID, 0, pngOfSize(100)); patched.Code != http.StatusNoContent {
		t.Fatalf("completing the first upload = %d %q; want 204", patched.Code, patched.Body.String())
	}
	if usage, err := store.DiskUsage(); err != nil || usage.ReservedBytes != 0 {
		t.Fatalf("DiskUsage() after completion = %+v, %v; want no reservation", usage, err)
	}
	if created := tusCreateUpload(resumable, secondID, 100); created.Code != http.StatusCreated {
		t.Fatalf("creation after the reservation was released = %d %q; want 201", created.Code, created.Body.String())
	}
}

func TestDiskUsageReportsOnlyBytesNotYetWritten(t *testing.T) {
	store := newReservationStore(t, 10_000)
	resumable := tusSession(t, store)
	if created := tusCreateUpload(resumable, testID, 200); created.Code != http.StatusCreated {
		t.Fatalf("creation = %d; want 201", created.Code)
	}
	// The 64-byte signature and 76 more bytes are written; 60 remain.
	if patched := tusPatchUpload(resumable, testID, 0, pngOfSize(200)[:140]); patched.Code != http.StatusNoContent {
		t.Fatalf("partial PATCH = %d %q; want 204", patched.Code, patched.Body.String())
	}
	usage, err := store.DiskUsage()
	if err != nil || usage.FreeBytes != 10_000 || usage.MinFreeBytes != 100 || usage.ReservedBytes != 60 {
		t.Fatalf("DiskUsage() during upload = %+v, %v; want free 10000, min 100, reserved 60", usage, err)
	}
}

// An abandoned upload keeps its reservation until its owner deletes it or
// reconciliation expires it; deleting it returns the reserved bytes.
func TestDeletedUploadReleasesItsReservation(t *testing.T) {
	store := newReservationStore(t, 250)
	resumable := tusSession(t, store)
	if created := tusCreateUpload(resumable, testID, 100); created.Code != http.StatusCreated {
		t.Fatalf("creation = %d; want 201", created.Code)
	}
	// Declares 100 bytes and sends 50: the rest stays reserved while the upload is pending.
	if patched := tusPatchUpload(resumable, testID, 0, pngOfSize(100)[:50]); patched.Code != http.StatusNoContent {
		t.Fatalf("partial PATCH = %d; want 204", patched.Code)
	}
	if usage, err := store.DiskUsage(); err != nil || usage.ReservedBytes != 50 {
		t.Fatalf("DiskUsage() with a pending upload = %+v, %v; want 50 reserved", usage, err)
	}

	if deleted := tusDeleteUpload(resumable, testID); deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d; want 204", deleted.Code)
	}
	if usage, err := store.DiskUsage(); err != nil || usage.ReservedBytes != 0 {
		t.Fatalf("DiskUsage() after DELETE = %+v, %v; want no reservation", usage, err)
	}
	if created := tusCreateUpload(resumable, secondID, 100); created.Code != http.StatusCreated {
		t.Fatalf("creation after DELETE = %d %q; want 201", created.Code, created.Body.String())
	}
	if entries, err := readDirWithoutTus(store.paths.Tmp); err != nil || len(entries) != 0 {
		t.Fatalf("tmp entries = %v, %v; want none", entries, err)
	}
}
