package media

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListHiddenReturnsOnlyHiddenMediaWithAdminPreviewURLs(t *testing.T) {
	gallery, database, paths := newTestGallery(t)
	insertMedia(t, database, idA, 4000, "visible")
	insertMedia(t, database, idB, 2000, "hidden")
	insertMedia(t, database, idC, 3000, "hidden")
	writeTestFile(t, filepath.Join(paths.Tmp, idC+"_t.staged"))
	writeTestFile(t, filepath.Join(paths.Tmp, idA+"_t.staged"))
	// A published file is never a hidden preview.
	writeTestFile(t, filepath.Join(paths.Derived, idB+"_t.jpg"))

	first, err := gallery.ListHidden(context.Background(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(first); len(got) != 1 || got[0] != idC || first.NextCursor == nil {
		t.Fatalf("first page = %v, cursor %v; want [%s] and a cursor", got, first.NextCursor, idC)
	}
	item := first.Items[0]
	if item.OriginalURL != nil || item.DetailURL != nil || item.ThumbURL == nil || *item.ThumbURL != "/api/admin/media/"+idC+"/preview/t" {
		t.Fatalf("item = %+v; want only the admin thumbnail preview URL", item)
	}
	encoded, err := json.Marshal(item)
	if err != nil || !strings.Contains(string(encoded), `"original_url":null`) {
		t.Fatalf("JSON = %s, %v; want original_url null", encoded, err)
	}

	second, err := gallery.ListHidden(context.Background(), *first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(second); len(got) != 1 || got[0] != idB || second.NextCursor != nil || second.Items[0].ThumbURL != nil {
		t.Fatalf("second page = %+v; want [%s] without previews and no cursor", second, idB)
	}
	if _, err := gallery.ListHidden(context.Background(), "!!!", 1); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("ListHidden(bad cursor) error = %v; want ErrInvalidCursor", err)
	}
}

func TestOpenHiddenPreviewServesOnlyStagedDerivativesOfHiddenMedia(t *testing.T) {
	gallery, database, paths := newTestGallery(t)
	insertMedia(t, database, idA, 1000, "visible")
	insertMedia(t, database, idB, 2000, "hidden")
	insertMedia(t, database, idC, 3000, "hidden")
	writeTestFile(t, filepath.Join(paths.Tmp, idA+"_t.staged"))
	writeTestFile(t, filepath.Join(paths.Tmp, idC+"_d.staged"))

	file, err := gallery.OpenHiddenPreview(context.Background(), idC, "d")
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(content) != "\xff\xd8\xff" {
		t.Fatalf("preview = %q, %v; want the staged JPEG", content, err)
	}
	for name, args := range map[string][2]string{
		"visible media":   {idA, "t"},
		"missing variant": {idB, "t"},
		"unknown variant": {idC, "x"},
		"unknown media":   {idD, "d"},
		"invalid id":      {"../" + idC, "d"},
	} {
		if _, err := gallery.OpenHiddenPreview(context.Background(), args[0], args[1]); !errors.Is(err, ErrPreviewNotFound) {
			t.Fatalf("%s error = %v; want ErrPreviewNotFound", name, err)
		}
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff}, 0600); err != nil {
		t.Fatal(err)
	}
}
