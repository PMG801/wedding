package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeClock is an injectable clock for carousel slots.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// slotBase is 18:00 UTC: a multiple of 30 minutes, so slot boundaries fall on
// 18:00, 18:30, 19:00 and so on.
var slotBase = time.Date(2026, 10, 18, 18, 0, 0, 0, time.UTC)

func testID(n int) string { return fmt.Sprintf("%08x-0000-4000-8000-%012x", n, n) }

// capturedAt returns the local capture string n seconds after the window start.
func capturedAt(n int) string {
	return slotBase.Add(time.Duration(n) * time.Second).Format("2006-01-02T15:04:05")
}

func insertCaptured(t *testing.T, database *sql.DB, id string, visibility string, captured any) {
	t.Helper()
	_, err := database.Exec("INSERT INTO media (id, storage_name, media_type, size_bytes, confirmed_at, visibility, captured_at) VALUES (?, ?, 'image/jpeg', 3, 1000, ?, ?)",
		id, "name_"+id+".jpg", visibility, captured)
	if err != nil {
		t.Fatal(err)
	}
}

func newTestCarousel(t *testing.T, periods []Period, size int, clock *fakeClock) (*Carousel, *sql.DB) {
	t.Helper()
	gallery, database, _ := newTestGallery(t)
	carousel, err := NewCarousel(gallery, CarouselConfig{Periods: periods, Size: size, Rotation: 30 * time.Minute, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return carousel, database
}

var window18to20 = []Period{{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T20:00"}}

func TestStripPicksAtMostSizeVisibleItemsOfThePeriodInCaptureOrder(t *testing.T) {
	clock := &fakeClock{now: slotBase}
	carousel, database := newTestCarousel(t, window18to20, 5, clock)
	for n := 1; n <= 40; n++ {
		insertCaptured(t, database, testID(n), "visible", capturedAt(n*60))
	}
	insertCaptured(t, database, testID(900), "hidden", capturedAt(100))
	insertCaptured(t, database, testID(901), "visible", nil)
	insertCaptured(t, database, testID(902), "visible", slotBase.Add(2*time.Hour).Format("2006-01-02T15:04:05"))

	strip, err := carousel.Strip(context.Background(), "guest-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(strip.Periods) != 1 {
		t.Fatalf("periods = %+v; want one bubble", strip.Periods)
	}
	bubble := strip.Periods[0]
	if bubble.ID != 1 || bubble.Start != "2026-10-18T18:00" || bubble.End != "2026-10-18T20:00" {
		t.Fatalf("bubble = %+v; want period 1 bounds", bubble)
	}
	if bubble.Total != 40 {
		t.Fatalf("total = %d; want 40 visible media in the window (hidden, uncaptured and outside excluded)", bubble.Total)
	}
	got := ids(Page{Items: bubble.Items})
	if len(got) != 5 {
		t.Fatalf("items = %d; want 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("items %v not in capture order; testID order follows capture order", got)
		}
	}
	for _, id := range got {
		if id == testID(900) || id == testID(901) || id == testID(902) {
			t.Fatalf("item %s must never appear", id)
		}
	}
	if bubble.Items[0].OriginalURL == nil {
		t.Fatal("item original URL is nil; want the public original URL like the gallery")
	}
}

func TestStripIsIdenticalForEveryGuestWithinASlotAndChangesNextSlot(t *testing.T) {
	clock := &fakeClock{now: slotBase}
	carousel, database := newTestCarousel(t, window18to20, 4, clock)
	for n := 1; n <= 30; n++ {
		insertCaptured(t, database, testID(n), "visible", capturedAt(n*60))
	}
	ctx := context.Background()
	first, err := carousel.Strip(ctx, "guest-a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := carousel.Strip(ctx, "guest-b")
	if err != nil {
		t.Fatal(err)
	}
	again := func(s Strip) []string { return ids(Page{Items: s.Periods[0].Items}) }
	if fmt.Sprint(again(first)) != fmt.Sprint(again(other)) {
		t.Fatalf("guests see %v and %v in the same slot; want identical", again(first), again(other))
	}

	clock.now = slotBase.Add(29*time.Minute + 59*time.Second)
	sameSlot, err := carousel.Strip(ctx, "guest-c")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(again(sameSlot)) != fmt.Sprint(again(first)) {
		t.Fatalf("selection changed inside the slot: %v then %v", again(first), again(sameSlot))
	}

	clock.now = slotBase.Add(30 * time.Minute)
	nextSlot, err := carousel.Strip(ctx, "guest-a")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(again(nextSlot)) == fmt.Sprint(again(first)) {
		t.Fatalf("selection %v did not rotate into the next slot", again(nextSlot))
	}
}

func TestStripRotatesAtIsTheStartOfTheNextSlotInUnixMilliseconds(t *testing.T) {
	clock := &fakeClock{now: slotBase.Add(10 * time.Minute)}
	carousel, _ := newTestCarousel(t, window18to20, 3, clock)
	for _, test := range []struct {
		now  time.Time
		want time.Time
	}{
		{slotBase, slotBase.Add(30 * time.Minute)},
		{slotBase.Add(29*time.Minute + 59*time.Second), slotBase.Add(30 * time.Minute)},
		{slotBase.Add(30 * time.Minute), slotBase.Add(60 * time.Minute)},
	} {
		clock.now = test.now
		strip, err := carousel.Strip(context.Background(), "guest")
		if err != nil {
			t.Fatal(err)
		}
		if want := test.want.UnixMilli(); strip.RotatesAt != want {
			t.Errorf("rotates_at at %v = %d; want %d", test.now, strip.RotatesAt, want)
		}
	}
}

func TestStripOmitsPeriodsWithoutVisibleMediaAndIsEmptyWithoutPeriods(t *testing.T) {
	periods := []Period{
		{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T19:00"},
		{ID: 2, Start: "2026-10-18T19:00", End: "2026-10-18T20:00"},
	}
	clock := &fakeClock{now: slotBase}
	carousel, database := newTestCarousel(t, periods, 3, clock)
	insertCaptured(t, database, testID(1), "hidden", capturedAt(60))
	insertCaptured(t, database, testID(2), "visible", capturedAt(3600+60))

	strip, err := carousel.Strip(context.Background(), "guest")
	if err != nil {
		t.Fatal(err)
	}
	if len(strip.Periods) != 1 || strip.Periods[0].ID != 2 {
		t.Fatalf("periods = %+v; want only period 2, the one with visible media", strip.Periods)
	}

	empty, err := NewCarousel(nil, CarouselConfig{})
	if err == nil || empty != nil {
		t.Fatal("NewCarousel without a gallery succeeded; want an error")
	}
	off, _ := newTestCarousel(t, nil, 3, clock)
	strip, err = off.Strip(context.Background(), "guest")
	if err != nil || strip.Periods == nil || len(strip.Periods) != 0 {
		t.Fatalf("feature off strip = %+v, %v; want an empty, non-nil periods list", strip, err)
	}
}

func TestAlbumPagesVisibleMediaOfThePeriodAscendingWithKeysetCursor(t *testing.T) {
	clock := &fakeClock{now: slotBase}
	carousel, database := newTestCarousel(t, window18to20, 3, clock)
	insertCaptured(t, database, testID(3), "visible", capturedAt(600))
	insertCaptured(t, database, testID(2), "visible", capturedAt(0))
	insertCaptured(t, database, testID(1), "visible", capturedAt(0))
	insertCaptured(t, database, testID(4), "visible", capturedAt(7199))
	insertCaptured(t, database, testID(5), "visible", slotBase.Add(2*time.Hour).Format("2006-01-02T15:04:05"))
	insertCaptured(t, database, testID(6), "hidden", capturedAt(300))
	insertCaptured(t, database, testID(7), "visible", nil)

	ctx := context.Background()
	first, err := carousel.Album(ctx, "guest", 1, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(first); fmt.Sprint(got) != fmt.Sprint([]string{testID(1), testID(2)}) {
		t.Fatalf("first album page = %v; want the same-second pair ordered by id", got)
	}
	if first.NextCursor == nil {
		t.Fatal("first album page has no next cursor")
	}
	second, err := carousel.Album(ctx, "guest", 1, *first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(second); fmt.Sprint(got) != fmt.Sprint([]string{testID(3), testID(4)}) {
		t.Fatalf("second album page = %v; want [%s %s]", got, testID(3), testID(4))
	}
	if second.NextCursor != nil {
		t.Fatalf("last album page next cursor = %q; want nil", *second.NextCursor)
	}
}

func TestAlbumRejectsUnknownPeriodAndInvalidCursor(t *testing.T) {
	carousel, _ := newTestCarousel(t, window18to20, 3, &fakeClock{now: slotBase})
	if _, err := carousel.Album(context.Background(), "guest", 9, "", 50); !errors.Is(err, ErrPeriodNotFound) {
		t.Fatalf("unknown period error = %v; want ErrPeriodNotFound", err)
	}
	for _, cursor := range []string{"!!", encodeCaptureCursor("2026-02-30T10:00:00", testID(1)), encodeCaptureCursor("2026-10-18T18:00:00", "not-an-id")} {
		if _, err := carousel.Album(context.Background(), "guest", 1, cursor, 50); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor %q error = %v; want ErrInvalidCursor", cursor, err)
		}
	}
}

// albumPlan returns the EXPLAIN QUERY PLAN detail lines for the album query.
func albumPlan(t *testing.T, database *sql.DB, query string, args []any) []string {
	t.Helper()
	rows, err := database.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestAlbumPagingUsesTheCapturedIndexWithoutSorting(t *testing.T) {
	_, database := newTestCarousel(t, window18to20, 3, &fakeClock{now: slotBase})
	period := window18to20[0]
	for _, withCursor := range []bool{false, true} {
		cursorCaptured, cursorID := "", ""
		if withCursor {
			cursorCaptured, cursorID = capturedAt(60), testID(1)
		}
		plan := strings.Join(albumPlan(t, database, albumSQL(withCursor), albumArgs("guest", period, cursorCaptured, cursorID, 50)), "\n")
		t.Logf("EXPLAIN QUERY PLAN album (cursor=%v):\n%s", withCursor, plan)
		if !strings.Contains(plan, "media_visible_captured") {
			t.Errorf("album query (cursor=%v) plan does not use media_visible_captured:\n%s", withCursor, plan)
		}
		if strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("album query (cursor=%v) plan sorts in a temporary b-tree:\n%s", withCursor, plan)
		}
	}
}
