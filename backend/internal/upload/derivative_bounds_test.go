package upload

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveDerivativeUsesItsOwnLimitBeforeReading(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxDerivativeBytes: 10, MaxConcurrent: 1})
	if _, err := store.SaveDerivative(context.Background(), testID, "t", 11, errorReader{}); !errors.Is(err, ErrDerivativeTooLarge) {
		t.Fatalf("SaveDerivative(11 bytes) error = %v; want ErrDerivativeTooLarge", err)
	}
	assertNoDerivativeArtifacts(t, paths)
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3, 4, 5, 6, 7}
	if _, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(jpeg)), bytes.NewReader(jpeg)); err != nil {
		t.Fatalf("SaveDerivative at the limit = %v", err)
	}
	// Photos keep MaxBytes.
	if _, err := store.SavePhoto(context.Background(), secondID, 100, bytes.NewReader(pngOfSize(100))); err != nil {
		t.Fatalf("SavePhoto above the derivative limit = %v", err)
	}
}

func TestReconcileDeletesExpiredStagedDerivativesOfUnconfirmedOriginals(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1, StagedDerivativeTTL: time.Hour})
	old := time.Now().Add(-2 * time.Hour)

	expired := filepath.Join(paths.Tmp, testID+"_t.staged")
	writeFile(t, expired, "old")
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(paths.Tmp, secondID+"_t.staged")
	writeFile(t, fresh, "new")

	// A hidden original keeps its old staged derivative: it may be shown again.
	if _, err := store.db.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES (?, 'x.jpg', 'image/jpeg', 1, 1, 'hidden')", thirdID); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(paths.Tmp, thirdID+"_d.staged")
	writeFile(t, hidden, "hidden")
	if err := os.Chtimes(hidden, old, old); err != nil {
		t.Fatal(err)
	}

	report, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() = %v", err)
	}
	if report.RemovedStaged != 1 {
		t.Fatalf("RemovedStaged = %d; want 1", report.RemovedStaged)
	}
	assertMissing(t, expired)
	assertContent(t, fresh, "new")
	assertContent(t, hidden, "hidden")
}

func TestReconcileKeepsStagedDerivativesWithoutTTL(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	staged := filepath.Join(paths.Tmp, testID+"_t.staged")
	writeFile(t, staged, "old")
	old := time.Now().Add(-1000 * time.Hour)
	if err := os.Chtimes(staged, old, old); err != nil {
		t.Fatal(err)
	}
	if report, err := store.Reconcile(context.Background()); err != nil || report.RemovedStaged != 0 {
		t.Fatalf("Reconcile() = %+v, %v; want nothing removed", report, err)
	}
	assertContent(t, staged, "old")
}
