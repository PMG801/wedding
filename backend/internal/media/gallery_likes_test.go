package media

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

const idF = "ffffffff-ffff-4fff-8fff-ffffffffffff"

func guestN(n int) string { return fmt.Sprintf("%032x", n) }

// likeBy records one like per guest number n for the medium.
func likeBy(t *testing.T, likes *Likes, id string, guests ...int) {
	t.Helper()
	for _, n := range guests {
		if _, err := likes.Like(context.Background(), guestN(n), id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListReportsLikeCountAndLikedByMePerGuest(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	likes := mustLikes(t, database)
	insertMedia(t, database, idA, 1000, "visible")
	insertMedia(t, database, idB, 2000, "visible")
	likeBy(t, likes, idA, 1, 2)
	likeBy(t, likes, idB, 2)

	page, err := gallery.ListForGuest(context.Background(), guestN(1), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page); len(got) != 2 || got[0] != idB || got[1] != idA {
		t.Fatalf("chronological IDs = %v; want [%s %s]", got, idB, idA)
	}
	b, a := page.Items[0], page.Items[1]
	if b.LikeCount != 1 || b.LikedByMe {
		t.Errorf("B for guest 1 = count %d liked %v; want 1 false", b.LikeCount, b.LikedByMe)
	}
	if a.LikeCount != 2 || !a.LikedByMe {
		t.Errorf("A for guest 1 = count %d liked %v; want 2 true", a.LikeCount, a.LikedByMe)
	}

	other, err := gallery.ListForGuest(context.Background(), guestN(3), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range other.Items {
		if item.LikedByMe {
			t.Errorf("guest 3 sees liked_by_me on %s; want false", item.ID)
		}
	}
	if other.Items[1].LikeCount != 2 {
		t.Errorf("A count for guest 3 = %d; want 2", other.Items[1].LikeCount)
	}
}

func TestListKeepsCursorStableWhenLikesChangeBetweenPages(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	likes := mustLikes(t, database)
	insertMedia(t, database, idA, 1000, "visible")
	insertMedia(t, database, idB, 2000, "visible")
	insertMedia(t, database, idC, 3000, "visible")

	first, err := gallery.ListForGuest(context.Background(), guestN(1), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Likes on the item of the next page must not shift the cursor.
	likeBy(t, likes, idB, 1, 2, 3)
	second, err := gallery.ListForGuest(context.Background(), guestN(1), *first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(second); len(got) != 1 || got[0] != idB || second.Items[0].LikeCount != 3 || !second.Items[0].LikedByMe {
		t.Fatalf("second page = %+v; want B with count 3 liked", second.Items)
	}
}

func TestListHiddenReportsLikeCountWithoutLikedByMe(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	insertMedia(t, database, idA, 1000, "hidden")
	if _, err := database.Exec("UPDATE media SET like_count = 1 WHERE id = ?", idA); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO media_likes (media_id, guest_id, created_at) VALUES (?, ?, 1)", idA, guestN(1)); err != nil {
		t.Fatal(err)
	}

	page, err := gallery.ListHidden(context.Background(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].LikeCount != 1 || page.Items[0].LikedByMe {
		t.Fatalf("hidden page = %+v; want one item with count 1 and liked_by_me false", page.Items)
	}
}

func TestTopListsOnlyVisibleLikedMediaByPopularityWithTies(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	likes := mustLikes(t, database)
	insertMedia(t, database, idA, 1000, "visible") // 3 likes, oldest of the tie
	insertMedia(t, database, idB, 2000, "visible") // 3 likes
	insertMedia(t, database, idC, 2000, "visible") // 3 likes, same time as B; higher id
	insertMedia(t, database, idD, 3000, "visible") // 2 likes
	insertMedia(t, database, idE, 4000, "visible") // no likes: excluded
	insertMedia(t, database, idF, 5000, "hidden")  // hidden: excluded even with likes
	likeBy(t, likes, idA, 1, 2, 3)
	likeBy(t, likes, idB, 1, 2, 3)
	likeBy(t, likes, idC, 1, 2, 3)
	likeBy(t, likes, idD, 1, 2)
	// Likes received before hiding stay stored; hidden media cannot gain new ones.
	for n := 1; n <= 5; n++ {
		if _, err := database.Exec("INSERT INTO media_likes (media_id, guest_id, created_at) VALUES (?, ?, 1)", idF, guestN(n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec("UPDATE media SET like_count = 5 WHERE id = ?", idF); err != nil {
		t.Fatal(err)
	}

	top, err := gallery.Top(context.Background(), guestN(2))
	if err != nil {
		t.Fatal(err)
	}
	got := itemIDs(top)
	want := []string{idC, idB, idA, idD}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top IDs = %v; want %v", got, want)
	}
	wantCounts := []int64{3, 3, 3, 2}
	for i, item := range top {
		if item.LikeCount != wantCounts[i] || !item.LikedByMe {
			t.Fatalf("top[%d] = count %d liked %v; want count %d liked true", i, item.LikeCount, item.LikedByMe, wantCounts[i])
		}
	}
}

func TestTopIsCappedAtFiftyAndEmptyWithoutLikes(t *testing.T) {
	gallery, database, _ := newTestGallery(t)
	empty, err := gallery.Top(context.Background(), guestN(1))
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty top = %#v; want non-nil empty slice", empty)
	}

	// Aggregates are set directly: Top reads only like_count, and the cap is what is under test.
	for i := 1; i <= 60; i++ {
		id := fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i)
		insertMedia(t, database, id, int64(i), "visible")
		if _, err := database.Exec("UPDATE media SET like_count = ? WHERE id = ?", i, id); err != nil {
			t.Fatal(err)
		}
	}
	top, err := gallery.Top(context.Background(), guestN(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 50 {
		t.Fatalf("top length = %d; want 50", len(top))
	}
	if top[0].LikeCount != 60 || top[49].LikeCount != 11 {
		t.Fatalf("top counts from %d to %d; want 60 down to 11", top[0].LikeCount, top[49].LikeCount)
	}
	for i := 1; i < len(top); i++ {
		if top[i-1].LikeCount < top[i].LikeCount {
			t.Fatalf("top not sorted by like_count at %d", i)
		}
	}
}

func TestTopUsesPopularityIndex(t *testing.T) {
	_, database, _ := newTestGallery(t)
	rows, err := database.Query("EXPLAIN QUERY PLAN "+topQuery, guestN(1), topLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if !strings.Contains(plan.String(), "media_visible_popularity") {
		t.Fatalf("top query plan does not use media_visible_popularity:\n%s", plan.String())
	}
}

func mustLikes(t *testing.T, database *sql.DB) *Likes {
	t.Helper()
	likes, err := NewLikes(database)
	if err != nil {
		t.Fatal(err)
	}
	return likes
}

func itemIDs(items []Item) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}
