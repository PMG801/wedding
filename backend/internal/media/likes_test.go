package media

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
)

const (
	guestOne   = "11111111111111111111111111111111"
	guestTwo   = "22222222222222222222222222222222"
	guestThree = "33333333333333333333333333333333"
)

func TestLikeIsIdempotentPerGuestAndCountsOnce(t *testing.T) {
	likes, database := newTestLikes(t)
	insertMedia(t, database, idA, 1000, "visible")
	ctx := context.Background()

	first, err := likes.Like(ctx, guestOne, idA)
	if err != nil {
		t.Fatal(err)
	}
	if first != (LikeState{ID: idA, LikeCount: 1, LikedByMe: true}) {
		t.Fatalf("first like = %+v; want count 1 liked", first)
	}
	again, err := likes.Like(ctx, guestOne, idA)
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Fatalf("repeated like = %+v; want unchanged %+v", again, first)
	}
	assertLikeState(t, database, idA, 1, 1)
}

func TestUnlikeIsIdempotentAndOnlyRemovesOwnLike(t *testing.T) {
	likes, database := newTestLikes(t)
	insertMedia(t, database, idA, 1000, "visible")
	ctx := context.Background()
	if _, err := likes.Like(ctx, guestOne, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := likes.Like(ctx, guestTwo, idA); err != nil {
		t.Fatal(err)
	}

	removed, err := likes.Unlike(ctx, guestOne, idA)
	if err != nil {
		t.Fatal(err)
	}
	if removed != (LikeState{ID: idA, LikeCount: 1, LikedByMe: false}) {
		t.Fatalf("unlike = %+v; want count 1 not liked", removed)
	}
	again, err := likes.Unlike(ctx, guestOne, idA)
	if err != nil {
		t.Fatal(err)
	}
	if again != removed {
		t.Fatalf("repeated unlike = %+v; want unchanged %+v", again, removed)
	}
	neverLiked, err := likes.Unlike(ctx, guestThree, idA)
	if err != nil {
		t.Fatal(err)
	}
	if neverLiked.LikeCount != 1 || neverLiked.LikedByMe {
		t.Fatalf("unlike by a guest without a like = %+v; want count 1 and not liked", neverLiked)
	}
	assertLikeState(t, database, idA, 1, 1)
}

func TestDistinctGuestsAreDistinctVoters(t *testing.T) {
	likes, database := newTestLikes(t)
	insertMedia(t, database, idA, 1000, "visible")
	ctx := context.Background()
	for _, guest := range []string{guestOne, guestTwo} {
		if _, err := likes.Like(ctx, guest, idA); err != nil {
			t.Fatal(err)
		}
	}
	assertLikeState(t, database, idA, 2, 2)
}

func TestHiddenOrUnknownMediaIsNotFoundAndLeavesLikesUnchanged(t *testing.T) {
	likes, database := newTestLikes(t)
	insertMedia(t, database, idA, 1000, "hidden")
	ctx := context.Background()
	// A like recorded before hiding is kept, but must not change while hidden.
	if _, err := database.Exec("INSERT INTO media_likes (media_id, guest_id, created_at) VALUES (?, ?, 1)", idA, guestOne); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE media SET like_count = 1 WHERE id = ?", idA); err != nil {
		t.Fatal(err)
	}

	if _, err := likes.Like(ctx, guestTwo, idA); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("like hidden media error = %v; want ErrMediaNotFound", err)
	}
	if _, err := likes.Unlike(ctx, guestOne, idA); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("unlike hidden media error = %v; want ErrMediaNotFound", err)
	}
	if _, err := likes.Like(ctx, guestTwo, idE); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("like unknown media error = %v; want ErrMediaNotFound", err)
	}
	if _, err := likes.Unlike(ctx, guestOne, idE); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("unlike unknown media error = %v; want ErrMediaNotFound", err)
	}
	assertLikeState(t, database, idA, 1, 1)
}

func TestLikeCountSurvivesReopenAndMatchesLikeRows(t *testing.T) {
	ctx := context.Background()
	paths, err := storage.Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(paths.Data, "reopen.db")
	database, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	insertMedia(t, database, idA, 1000, "visible")
	insertMedia(t, database, idB, 2000, "visible")
	likes, err := NewLikes(database)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		guest, id string
		like      bool
	}{
		{guestOne, idA, true}, {guestTwo, idA, true}, {guestOne, idA, true},
		{guestOne, idB, true}, {guestTwo, idA, false}, {guestThree, idA, false},
	} {
		var err error
		if step.like {
			_, err = likes.Like(ctx, step.guest, step.id)
		} else {
			_, err = likes.Unlike(ctx, step.guest, step.id)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertLikeState(t, reopened, idA, 1, 1)
	assertLikeState(t, reopened, idB, 1, 1)
}

func newTestLikes(t *testing.T) (*Likes, *sql.DB) {
	t.Helper()
	_, database, _ := newTestGallery(t)
	likes, err := NewLikes(database)
	if err != nil {
		t.Fatal(err)
	}
	return likes, database
}

// assertLikeState checks the aggregate column and the number of like rows for
// one medium; the two must always agree.
func assertLikeState(t *testing.T, database *sql.DB, id string, wantCount, wantRows int) {
	t.Helper()
	var count, rows int
	if err := database.QueryRow("SELECT like_count FROM media WHERE id = ?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT count(*) FROM media_likes WHERE media_id = ?", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if count != wantCount || rows != wantRows {
		t.Fatalf("media %s like_count = %d, like rows = %d; want %d and %d", id, count, rows, wantCount, wantRows)
	}
}
