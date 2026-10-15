package upload

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReconcileReportsDatabaseErrorsForStagedDerivatives(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	writeFile(t, filepath.Join(paths.Tmp, visibleID+"_t.staged"), jpegHeader+"t")
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile error = nil; want the database failure reported")
	}
	assertContent(t, filepath.Join(paths.Tmp, visibleID+"_t.staged"), jpegHeader+"t")
}
