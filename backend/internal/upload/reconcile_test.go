package upload

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

const (
	activeID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	orphanID   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	visibleID  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	hiddenID   = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	pendingID  = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	jpegHeader = "\xff\xd8\xff"
)

func TestReconcileRemovesAbandonedPartialsButProtectsActiveUploads(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	writeFile(t, filepath.Join(paths.Tmp, orphanID+".part"), "abandoned")
	writeFile(t, filepath.Join(paths.Tmp, orphanID+"_t.part"), "abandoned")
	writeFile(t, filepath.Join(paths.Tmp, activeID+".part"), "in progress")
	unlock, err := store.lockID(context.Background(), activeID)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	report, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertMissing(t, filepath.Join(paths.Tmp, orphanID+".part"))
	assertMissing(t, filepath.Join(paths.Tmp, orphanID+"_t.part"))
	assertContent(t, filepath.Join(paths.Tmp, activeID+".part"), "in progress")
	if report.RemovedPartials != 2 || report.SkippedActive != 1 {
		t.Fatalf("report = %+v; want 2 removed partials and 1 skipped active upload", report)
	}
}

func TestReconcileQuarantinesUnindexedOriginalsWithoutDeleting(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	indexed, err := store.SavePhoto(context.Background(), visibleID, 4, bytes.NewReader([]byte(jpegHeader+"1")))
	if err != nil {
		t.Fatal(err)
	}
	orphanName := "20260926T183012.000000000Z_" + orphanID + ".jpg"
	writeFile(t, filepath.Join(paths.Originals, orphanName), "orphan bytes")
	strayName := "notes.txt"
	writeFile(t, filepath.Join(paths.Originals, strayName), "stray")
	confirmingName := "20260926T183013.000000000Z_" + activeID + ".jpg"
	writeFile(t, filepath.Join(paths.Originals, confirmingName), "being confirmed")
	unlock, err := store.lockID(context.Background(), activeID)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	report, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertMissing(t, filepath.Join(paths.Originals, orphanName))
	assertContent(t, filepath.Join(paths.Quarantine, orphanName), "orphan bytes")
	assertMissing(t, filepath.Join(paths.Originals, strayName))
	assertContent(t, filepath.Join(paths.Quarantine, strayName), "stray")
	assertContent(t, filepath.Join(paths.Originals, confirmingName), "being confirmed")
	assertContent(t, filepath.Join(paths.Originals, indexed.StorageName), jpegHeader+"1")
	if report.Quarantined != 2 || report.SkippedActive != 1 {
		t.Fatalf("report = %+v; want 2 quarantined and 1 skipped active upload", report)
	}
}

func TestReconcileNeverOverwritesQuarantinedFiles(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := "20260926T183012.000000000Z_" + orphanID + ".jpg"
	writeFile(t, filepath.Join(paths.Quarantine, name), "first copy")
	writeFile(t, filepath.Join(paths.Originals, name), "second copy")

	if _, err := store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filepath.Join(paths.Quarantine, name), "first copy")
	entries, err := os.ReadDir(paths.Quarantine)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("quarantine holds %d files; want both copies kept", len(entries))
	}
	assertMissing(t, filepath.Join(paths.Originals, name))
}

func TestReconcilePromotesStagedDerivativesOnlyForVisibleOriginals(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	for _, id := range []string{visibleID, hiddenID} {
		if _, err := store.SavePhoto(context.Background(), id, 4, bytes.NewReader([]byte(jpegHeader+"1"))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec("UPDATE media SET visibility = 'hidden' WHERE id = ?", hiddenID); err != nil {
		t.Fatal(err)
	}
	// A promotion that failed after confirmation leaves the variant staged.
	for _, id := range []string{visibleID, hiddenID, pendingID} {
		writeFile(t, filepath.Join(paths.Tmp, id+"_d.staged"), jpegHeader+"d")
	}

	report, err := store.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertContent(t, filepath.Join(paths.Derived, visibleID+"_d.jpg"), jpegHeader+"d")
	assertMissing(t, filepath.Join(paths.Tmp, visibleID+"_d.staged"))
	assertContent(t, filepath.Join(paths.Tmp, hiddenID+"_d.staged"), jpegHeader+"d")
	assertMissing(t, filepath.Join(paths.Derived, hiddenID+"_d.jpg"))
	assertContent(t, filepath.Join(paths.Tmp, pendingID+"_d.staged"), jpegHeader+"d")
	if report.PromotedDerivatives != 1 {
		t.Fatalf("report = %+v; want 1 promoted derivative", report)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists or cannot be inspected (%v); want it removed", path, err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q; want %q", path, got, want)
	}
}
