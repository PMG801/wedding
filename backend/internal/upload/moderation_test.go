package upload

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PMG801/wedding/internal/storage"
)

// moderatedPhoto stores a confirmed photo with both derivatives published and
// returns its storage name.
func moderatedPhoto(t *testing.T, store *Store) string {
	t.Helper()
	media, err := store.SavePhoto(context.Background(), testID, 4, bytes.NewReader([]byte(jpegHeader+"o")))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"t", "d"} {
		body := []byte(jpegHeader + variant)
		if _, err := store.SaveDerivative(context.Background(), testID, variant, int64(len(body)), bytes.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	return media.StorageName
}

func assertVisibility(t *testing.T, store *Store, want string) {
	t.Helper()
	var got string
	if err := store.db.QueryRow("SELECT visibility FROM media WHERE id = ?", testID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("visibility = %q; want %q", got, want)
	}
}

func assertServed(t *testing.T, paths storage.Paths, name string) {
	t.Helper()
	assertContent(t, filepath.Join(paths.Originals, name), jpegHeader+"o")
	assertMissing(t, filepath.Join(paths.Hidden, name))
	for _, variant := range []string{"t", "d"} {
		assertContent(t, filepath.Join(paths.Derived, testID+"_"+variant+".jpg"), jpegHeader+variant)
		assertMissing(t, filepath.Join(paths.Tmp, testID+"_"+variant+".staged"))
	}
}

func assertPrivate(t *testing.T, paths storage.Paths, name string) {
	t.Helper()
	assertContent(t, filepath.Join(paths.Hidden, name), jpegHeader+"o")
	assertMissing(t, filepath.Join(paths.Originals, name))
	for _, variant := range []string{"t", "d"} {
		assertContent(t, filepath.Join(paths.Tmp, testID+"_"+variant+".staged"), jpegHeader+variant)
		assertMissing(t, filepath.Join(paths.Derived, testID+"_"+variant+".jpg"))
	}
}

func TestSetVisibilityHidesAndShowsOriginalAndDerivatives(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := moderatedPhoto(t, store)

	hidden, err := store.SetVisibility(context.Background(), testID, "hidden")
	if err != nil || hidden.ID != testID || hidden.Visibility != "hidden" {
		t.Fatalf("hide = %+v, %v", hidden, err)
	}
	assertVisibility(t, store, "hidden")
	assertPrivate(t, paths, name)

	shown, err := store.SetVisibility(context.Background(), testID, "visible")
	if err != nil || shown.Visibility != "visible" {
		t.Fatalf("show = %+v, %v", shown, err)
	}
	assertVisibility(t, store, "visible")
	assertServed(t, paths, name)
}

func TestSetVisibilityIsIdempotent(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := moderatedPhoto(t, store)

	if got, err := store.SetVisibility(context.Background(), testID, "visible"); err != nil || got.Visibility != "visible" {
		t.Fatalf("show visible = %+v, %v", got, err)
	}
	assertServed(t, paths, name)
	for range 2 {
		if got, err := store.SetVisibility(context.Background(), testID, "hidden"); err != nil || got.Visibility != "hidden" {
			t.Fatalf("hide = %+v, %v", got, err)
		}
	}
	assertVisibility(t, store, "hidden")
	assertPrivate(t, paths, name)
}

func TestSetVisibilityRejectsUnknownMediaAndInvalidValuesWithoutChanges(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := moderatedPhoto(t, store)

	for _, id := range []string{visibleID, "not-a-uuid"} {
		if _, err := store.SetVisibility(context.Background(), id, "hidden"); !errors.Is(err, ErrMediaNotFound) {
			t.Fatalf("SetVisibility(%q) error = %v; want ErrMediaNotFound", id, err)
		}
	}
	for _, value := range []string{"", "deleted", "HIDDEN"} {
		if _, err := store.SetVisibility(context.Background(), testID, value); !errors.Is(err, ErrInvalidVisibility) {
			t.Fatalf("SetVisibility(%q) error = %v; want ErrInvalidVisibility", value, err)
		}
	}
	assertVisibility(t, store, "visible")
	assertServed(t, paths, name)
}

func TestSetVisibilityRollsBackFilesWhenHidingCannotBeRecorded(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := moderatedPhoto(t, store)
	if _, err := store.db.Exec("CREATE TRIGGER block_update BEFORE UPDATE ON media BEGIN SELECT RAISE(ABORT, 'blocked'); END"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SetVisibility(context.Background(), testID, "hidden"); err == nil {
		t.Fatal("hide succeeded although SQLite rejected the update")
	}
	assertVisibility(t, store, "visible")
	assertServed(t, paths, name)
}

func TestSetVisibilityKeepsMediaHiddenWhenShowingCannotMoveTheOriginal(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	name := moderatedPhoto(t, store)
	if _, err := store.SetVisibility(context.Background(), testID, "hidden"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.Originals, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(paths.Originals, 0700) })

	if _, err := store.SetVisibility(context.Background(), testID, "visible"); err == nil {
		t.Fatal("show succeeded although the original could not be moved")
	}
	assertVisibility(t, store, "hidden")
	assertPrivate(t, paths, name)
}
