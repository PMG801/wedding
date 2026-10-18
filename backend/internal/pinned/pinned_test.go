package pinned

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

const (
	idVisible  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	idHidden   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	idNoThumb  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	idUnknown  = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	idBadMedia = "not-a-uuid"
)

func newTestStore(t *testing.T) (*Store, *sql.DB, storage.Paths) {
	t.Helper()
	ctx := context.Background()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(ctx, filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := New(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	insertMedia(t, database, idVisible, "visible")
	insertMedia(t, database, idHidden, "hidden")
	insertMedia(t, database, idNoThumb, "visible")
	return store, database, paths
}

func insertMedia(t *testing.T, database *sql.DB, id, visibility string) {
	t.Helper()
	if _, err := database.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility) VALUES (?, ?, 'image/jpeg', 3, 1000, ?)",
		id, "name_"+id+".jpg", visibility); err != nil {
		t.Fatal(err)
	}
}

// publishThumb writes the published thumbnail file, which the disk decides on.
func publishThumb(t *testing.T, paths storage.Paths, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(paths.Derived, upload.DerivativeFileName(id, "t")), []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func publishDetail(t *testing.T, paths storage.Paths, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(paths.Derived, upload.DerivativeFileName(id, "d")), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ptr(s string) *string { return &s }

func mustCreate(t *testing.T, store *Store, title string) Item {
	t.Helper()
	item, err := store.Create(context.Background(), Input{Title: title, Body: "cuerpo"})
	if err != nil {
		t.Fatalf("Create(%q) = %v; want success", title, err)
	}
	return item
}

// rowsSnapshot returns every stored row in position order, so a rejected call
// can be proven to have changed nothing.
func rowsSnapshot(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query("SELECT id || '|' || title || '|' || body || '|' || coalesce(media_id, '-') || '|' || position || '|' || created_at || '|' || updated_at FROM pinned_items ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNewRequiresDatabase(t *testing.T) {
	if _, err := New(nil, storage.Paths{Derived: t.TempDir()}); err == nil {
		t.Fatal("New(nil) error = nil; want error")
	}
}

func TestCreateValidatesTitle(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	for _, test := range []struct {
		name  string
		title string
		want  error
		shown string
	}{
		{name: "empty", title: "", want: ErrInvalid},
		{name: "only spaces", title: "   ", want: ErrInvalid},
		{name: "120 ascii", title: strings.Repeat("a", 120), shown: strings.Repeat("a", 120)},
		{name: "121 ascii", title: strings.Repeat("a", 121), want: ErrInvalid},
		{name: "120 multibyte code points", title: strings.Repeat("ñ", 120), shown: strings.Repeat("ñ", 120)},
		{name: "121 multibyte code points", title: strings.Repeat("ñ", 121), want: ErrInvalid},
		{name: "trimmed", title: "  Horario  ", shown: "Horario"},
		{name: "trim does not count the spaces", title: "  " + strings.Repeat("a", 120) + "  ", shown: strings.Repeat("a", 120)},
		{name: "newline", title: "a\nb", want: ErrInvalid},
		{name: "tab", title: "a\tb", want: ErrInvalid},
		{name: "delete control", title: "a\x7fb", want: ErrInvalid},
		{name: "invalid utf-8", title: "a\xffb", want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := rowsSnapshot(t, database)
			item, err := store.Create(ctx, Input{Title: test.title, Body: ""})
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("Create error = %v; want %v", err, test.want)
				}
				if got := rowsSnapshot(t, database); !reflect.DeepEqual(got, before) {
					t.Fatalf("rejected create changed rows: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create error = %v; want success", err)
			}
			if item.Title != test.shown {
				t.Fatalf("stored title = %q; want %q", item.Title, test.shown)
			}
			if !upload.ValidID(item.ID) {
				t.Fatalf("id %q is not a UUIDv4", item.ID)
			}
			// Clean up so the next case starts at position 0 and below the limit.
			if err := store.Delete(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateValidatesBody(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	for _, test := range []struct {
		name string
		body string
		want error
	}{
		{name: "empty body is allowed", body: ""},
		{name: "2000 code points", body: strings.Repeat("ñ", 2000)},
		{name: "2001 code points", body: strings.Repeat("ñ", 2001), want: ErrInvalid},
		{name: "line breaks", body: "13:00 ceremonia\n14:30 aperitivo"},
		{name: "carriage return", body: "a\r\nb", want: ErrInvalid},
		{name: "tab", body: "a\tb", want: ErrInvalid},
		{name: "nul", body: "a\x00b", want: ErrInvalid},
		{name: "invalid utf-8", body: "a\xffb", want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := rowsSnapshot(t, database)
			item, err := store.Create(ctx, Input{Title: "Título", Body: test.body})
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("Create error = %v; want %v", err, test.want)
				}
				if got := rowsSnapshot(t, database); !reflect.DeepEqual(got, before) {
					t.Fatalf("rejected create changed rows: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create error = %v; want success", err)
			}
			if item.Body != test.body {
				t.Fatalf("stored body = %q; want the body unchanged", item.Body)
			}
			if err := store.Delete(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateValidatesMediaReference(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	for _, test := range []struct {
		name    string
		mediaID *string
		want    error
	}{
		{name: "no image", mediaID: nil},
		{name: "visible medium", mediaID: ptr(idVisible)},
		{name: "hidden medium is still referenceable", mediaID: ptr(idHidden)},
		{name: "unknown medium", mediaID: ptr(idUnknown), want: ErrInvalid},
		{name: "not a uuid", mediaID: ptr(idBadMedia), want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := rowsSnapshot(t, database)
			item, err := store.Create(ctx, Input{Title: "Aviso", MediaID: test.mediaID})
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("Create error = %v; want %v", err, test.want)
				}
				if got := rowsSnapshot(t, database); !reflect.DeepEqual(got, before) {
					t.Fatalf("rejected create changed rows: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create error = %v; want success", err)
			}
			if !reflect.DeepEqual(item.MediaID, test.mediaID) {
				t.Fatalf("MediaID = %v; want %v", item.MediaID, test.mediaID)
			}
			if err := store.Delete(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateAppendsAndRejectsSixthWithoutWriting(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	var ids []string
	for i := range 5 {
		item := mustCreate(t, store, "Entrada "+string(rune('A'+i)))
		if item.Position != i {
			t.Fatalf("item %d position = %d; want %d", i, item.Position, i)
		}
		ids = append(ids, item.ID)
	}
	before := rowsSnapshot(t, database)
	if _, err := store.Create(ctx, Input{Title: "Sexta"}); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("sixth Create error = %v; want ErrLimitReached", err)
	}
	if got := rowsSnapshot(t, database); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected sixth create changed rows:\n got %v\nwant %v", got, before)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range items {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, ids) {
		t.Fatalf("order after rejected create = %v; want %v", got, ids)
	}
}

func TestConcurrentCreatesNeverExceedLimit(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	const attempts = 12
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Create(ctx, Input{Title: "Concurrente " + string(rune('a'+i))})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	created, limited := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrLimitReached):
			limited++
		default:
			t.Fatalf("concurrent Create error = %v; want success or ErrLimitReached", err)
		}
	}
	if created != MaxItems || limited != attempts-MaxItems {
		t.Fatalf("created = %d, limited = %d; want %d and %d", created, limited, MaxItems, attempts-MaxItems)
	}
	var stored int
	if err := database.QueryRow("SELECT count(*) FROM pinned_items").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != MaxItems {
		t.Fatalf("stored rows = %d; want %d", stored, MaxItems)
	}
}

func TestUpdateReplacesFieldsAndKeepsPosition(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, store, "A")
	b := mustCreate(t, store, "B")
	mustCreate(t, store, "C")

	updated, err := store.Update(ctx, b.ID, Input{Title: "B editada", Body: "nuevo\ncuerpo", MediaID: ptr(idVisible)})
	if err != nil {
		t.Fatalf("Update() = %v", err)
	}
	if updated.ID != b.ID || updated.Position != 1 || updated.CreatedAt != b.CreatedAt {
		t.Fatalf("updated = %+v; want same id, position 1 and created_at %d", updated, b.CreatedAt)
	}
	if updated.Title != "B editada" || updated.Body != "nuevo\ncuerpo" || !reflect.DeepEqual(updated.MediaID, ptr(idVisible)) {
		t.Fatalf("updated fields = %+v", updated)
	}
	if updated.UpdatedAt < b.UpdatedAt {
		t.Fatalf("updated_at = %d; want at least %d", updated.UpdatedAt, b.UpdatedAt)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != a.ID || items[1].ID != b.ID || items[1].Position != 1 {
		t.Fatalf("order after update = %+v", items)
	}

	if _, err := store.Update(ctx, b.ID, Input{Title: strings.Repeat("x", 121)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Update with long title error = %v; want ErrInvalid", err)
	}
	if _, err := store.Update(ctx, b.ID, Input{Title: "ok", MediaID: ptr(idUnknown)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Update with unknown media error = %v; want ErrInvalid", err)
	}
	if _, err := store.Update(ctx, "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Input{Title: "ok"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update missing id error = %v; want ErrNotFound", err)
	}
	var stored string
	if err := database.QueryRow("SELECT title FROM pinned_items WHERE id = ?", b.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "B editada" {
		t.Fatalf("rejected updates changed title to %q", stored)
	}
}

func TestDeleteCompactsPositions(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, store, "A")
	b := mustCreate(t, store, "B")
	c := mustCreate(t, store, "C")
	d := mustCreate(t, store, "D")

	if err := store.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete(B) = %v", err)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id       string
		position int
	}{{a.ID, 0}, {c.ID, 1}, {d.ID, 2}}
	if len(items) != len(want) {
		t.Fatalf("items after delete = %d; want %d", len(items), len(want))
	}
	for i, w := range want {
		if items[i].ID != w.id || items[i].Position != w.position {
			t.Fatalf("item %d = %s@%d; want %s@%d", i, items[i].ID, items[i].Position, w.id, w.position)
		}
	}

	if err := store.Delete(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete error = %v; want ErrNotFound", err)
	}
	if err := store.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != c.ID || items[0].Position != 0 {
		t.Fatalf("after deleting first and last = %+v; want C at 0", items)
	}
}

func TestReorderRequiresExactUniqueSet(t *testing.T) {
	store, database, _ := newTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, store, "A")
	b := mustCreate(t, store, "B")
	c := mustCreate(t, store, "C")
	before := rowsSnapshot(t, database)

	for _, test := range []struct {
		name string
		ids  []string
	}{
		{name: "missing one", ids: []string{a.ID, b.ID}},
		{name: "extra unknown", ids: []string{a.ID, b.ID, c.ID, idUnknown}},
		{name: "duplicate", ids: []string{a.ID, a.ID, b.ID}},
		{name: "unknown instead of member", ids: []string{a.ID, b.ID, idUnknown}},
		{name: "empty with items", ids: []string{}},
		{name: "nil with items", ids: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Reorder(ctx, test.ids); !errors.Is(err, ErrInvalidOrder) {
				t.Fatalf("Reorder error = %v; want ErrInvalidOrder", err)
			}
			if got := rowsSnapshot(t, database); !reflect.DeepEqual(got, before) {
				t.Fatalf("rejected reorder changed rows:\n got %v\nwant %v", got, before)
			}
		})
	}

	items, err := store.Reorder(ctx, []string{c.ID, a.ID, b.ID})
	if err != nil {
		t.Fatalf("Reorder() = %v", err)
	}
	var got []string
	for i, item := range items {
		got = append(got, item.ID)
		if item.Position != i {
			t.Fatalf("returned item %d position = %d; want %d", i, item.Position, i)
		}
	}
	if !reflect.DeepEqual(got, []string{c.ID, a.ID, b.ID}) {
		t.Fatalf("reordered = %v", got)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listed, items) {
		t.Fatalf("List after reorder = %+v; want %+v", listed, items)
	}
}

func TestReorderEmptyStoreAcceptsEmptyList(t *testing.T) {
	store, _, _ := newTestStore(t)
	items, err := store.Reorder(context.Background(), []string{})
	if err != nil {
		t.Fatalf("Reorder([]) on empty store = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v; want none", items)
	}
}

func TestListResolvesImageOnlyForVisibleMediumWithThumbnail(t *testing.T) {
	store, database, paths := newTestStore(t)
	ctx := context.Background()
	publishThumb(t, paths, idVisible)
	publishDetail(t, paths, idVisible)
	publishThumb(t, paths, idHidden)
	// idNoThumb is visible but has no published thumbnail file.
	visible := mustCreate(t, store, "Visible")
	if _, err := store.Update(ctx, visible.ID, Input{Title: "Visible", MediaID: ptr(idVisible)}); err != nil {
		t.Fatal(err)
	}
	hidden := mustCreate(t, store, "Oculto")
	if _, err := store.Update(ctx, hidden.ID, Input{Title: "Oculto", MediaID: ptr(idHidden)}); err != nil {
		t.Fatal(err)
	}
	noThumb := mustCreate(t, store, "Sin miniatura")
	if _, err := store.Update(ctx, noThumb.ID, Input{Title: "Sin miniatura", MediaID: ptr(idNoThumb)}); err != nil {
		t.Fatal(err)
	}
	plain := mustCreate(t, store, "Sin imagen")

	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].ID != visible.ID || items[1].ID != hidden.ID || items[2].ID != noThumb.ID || items[3].ID != plain.ID {
		t.Fatalf("List order = %+v", items)
	}
	wantImage := &Image{ID: idVisible, ThumbURL: "/m/d/" + idVisible + "_t.jpg", DetailURL: ptr("/m/d/" + idVisible + "_d.jpg")}
	if !reflect.DeepEqual(items[0].Image, wantImage) {
		t.Fatalf("visible image = %+v; want %+v", items[0].Image, wantImage)
	}
	if items[1].Image != nil || items[2].Image != nil || items[3].Image != nil {
		t.Fatalf("hidden, thumbnail-less and plain images = %+v %+v %+v; want nil", items[1].Image, items[2].Image, items[3].Image)
	}

	// Hiding the visible medium changes the resolved image, never the entry.
	if _, err := database.Exec("UPDATE media SET visibility = 'hidden' WHERE id = ?", idVisible); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Image != nil || items[0].MediaID == nil || *items[0].MediaID != idVisible {
		t.Fatalf("after hiding: item = %+v; want entry kept without image", items[0])
	}
}

func TestListImageWithoutDetailHasNilDetailURL(t *testing.T) {
	store, _, paths := newTestStore(t)
	publishThumb(t, paths, idVisible)
	if _, err := store.Create(context.Background(), Input{Title: "Solo miniatura", MediaID: ptr(idVisible)}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Image == nil || items[0].Image.DetailURL != nil || items[0].Image.ThumbURL != "/m/d/"+idVisible+"_t.jpg" {
		t.Fatalf("items = %+v; want image with thumb and nil detail", items)
	}
}

func TestListEmptyStoreReturnsNoItems(t *testing.T) {
	store, _, _ := newTestStore(t)
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("List() = %#v; want empty non-nil slice", items)
	}
}
