package media

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
)

const (
	idA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	idB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	idC = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	idD = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	idE = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
)

func TestListReturnsVisibleMediaNewestFirstWithStableCursor(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	insertMedia(t, database, idA, 1000, "visible")
	insertMedia(t, database, idB, 2000, "visible")
	insertMedia(t, database, idC, 2000, "visible")
	insertMedia(t, database, idD, 3000, "hidden")

	first, err := gallery.List(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(first); len(got) != 2 || got[0] != idC || got[1] != idB {
		t.Fatalf("first page IDs = %v; want [%s %s]", got, idC, idB)
	}
	if first.NextCursor == nil {
		t.Fatal("first page next cursor is nil; want a cursor")
	}

	// A newer upload between page requests must not shift the next page.
	insertMedia(t, database, idE, 4000, "visible")
	second, err := gallery.List(context.Background(), *first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(second); len(got) != 1 || got[0] != idA {
		t.Fatalf("second page IDs = %v; want [%s]", got, idA)
	}
	if second.NextCursor != nil {
		t.Fatalf("last page next cursor = %q; want nil", *second.NextCursor)
	}

	item := second.Items[0]
	if item.MIME != "image/jpeg" || item.ConfirmedAt != 1000 || item.OriginalURL != "/m/o/name_"+idA+".jpg" {
		t.Fatalf("item = %+v; want stored MIME, confirmation time and original URL", item)
	}
}

func TestListReportsDerivativeURLsOnlyWhenPublishedOnDisk(t *testing.T) {
	gallery, database, paths := newTestGallery(t)
	insertMedia(t, database, idA, 1000, "visible")
	if err := os.WriteFile(filepath.Join(paths.Derived, idA+"_t.jpg"), []byte{0xff, 0xd8, 0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Tmp, idA+"_d.staged"), []byte{0xff, 0xd8, 0xff}, 0600); err != nil {
		t.Fatal(err)
	}

	page, err := gallery.List(context.Background(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	item := page.Items[0]
	if item.ThumbURL == nil || *item.ThumbURL != "/m/d/"+idA+"_t.jpg" {
		t.Fatalf("thumb URL = %v; want published thumbnail URL", item.ThumbURL)
	}
	if item.DetailURL != nil {
		t.Fatalf("detail URL = %q; want nil for a privately staged derivative", *item.DetailURL)
	}
}

func TestListEmptyGalleryReturnsEmptyItems(t *testing.T) {
	gallery, _, _ := newTestGallery(t)
	page, err := gallery.List(context.Background(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatalf("page = %+v; want non-nil empty items and no cursor", page)
	}
}

func TestListRejectsMalformedCursor(t *testing.T) {
	gallery, _, _ := newTestGallery(t)
	encode := base64.RawURLEncoding.EncodeToString
	for _, cursor := range []string{
		"!!!",
		encode([]byte("1000")),
		encode([]byte("x:" + idA)),
		encode([]byte("1000:not-a-uuid")),
	} {
		if _, err := gallery.List(context.Background(), cursor, 50); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("List(cursor %q) error = %v; want ErrInvalidCursor", cursor, err)
		}
	}
}

func newTestGallery(t *testing.T) (*Gallery, *sql.DB, storage.Paths) {
	t.Helper()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	gallery, err := NewGallery(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	return gallery, database, paths
}

func insertMedia(t *testing.T, database *sql.DB, id string, confirmedAt int64, visibility string) {
	t.Helper()
	_, err := database.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES (?, ?, 'image/jpeg', 3, ?, ?)",
		id, "name_"+id+".jpg", confirmedAt, visibility)
	if err != nil {
		t.Fatal(err)
	}
}

func ids(page Page) []string {
	result := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		result = append(result, item.ID)
	}
	return result
}
