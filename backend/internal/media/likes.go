package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrMediaNotFound reports a missing or hidden medium. Hidden media cannot be
// liked or unliked, and their like rows stay untouched.
var ErrMediaNotFound = errors.New("media not found")

// LikeState is the final state of one medium for one guest identity.
type LikeState struct {
	ID        string
	LikeCount int64
	LikedByMe bool
}

// Likes stores anonymous positive «me gusta» votes in SQLite (ADR-0012).
type Likes struct {
	db *sql.DB
}

// NewLikes builds the like store over an initialized database.
func NewLikes(database *sql.DB) (*Likes, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &Likes{db: database}, nil
}

// Like records one «me gusta» from guestID. Repeating it is idempotent: the
// row and the aggregate change only once.
func (l *Likes) Like(ctx context.Context, guestID, mediaID string) (LikeState, error) {
	return l.apply(ctx, guestID, mediaID, true)
}

// Unlike removes the «me gusta» of guestID. Repeating it is idempotent.
func (l *Likes) Unlike(ctx context.Context, guestID, mediaID string) (LikeState, error) {
	return l.apply(ctx, guestID, mediaID, false)
}

// apply changes the like row and the aggregate in one transaction. Each
// statement that writes runs first, so the transaction takes the write lock
// immediately and never needs a read-to-write upgrade.
func (l *Likes) apply(ctx context.Context, guestID, mediaID string, like bool) (LikeState, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return LikeState{}, fmt.Errorf("begin like transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if like {
		err = insertLike(ctx, tx, guestID, mediaID)
	} else {
		err = deleteLike(ctx, tx, guestID, mediaID)
	}
	if err != nil {
		return LikeState{}, err
	}

	var state LikeState
	state.ID = mediaID
	var visibility string
	err = tx.QueryRowContext(ctx,
		`SELECT m.visibility, m.like_count,
		        EXISTS (SELECT 1 FROM media_likes l WHERE l.media_id = m.id AND l.guest_id = ?)
		   FROM media m WHERE m.id = ?`, guestID, mediaID).Scan(&visibility, &state.LikeCount, &state.LikedByMe)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && visibility != "visible") {
		return LikeState{}, ErrMediaNotFound
	}
	if err != nil {
		return LikeState{}, fmt.Errorf("read like state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LikeState{}, fmt.Errorf("commit like transaction: %w", err)
	}
	return state, nil
}

// insertLike adds the row and increments the aggregate only when the row was
// actually new and the medium is visible.
func insertLike(ctx context.Context, tx *sql.Tx, guestID, mediaID string) error {
	result, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO media_likes (media_id, guest_id, created_at)
		 SELECT id, ?, ? FROM media WHERE id = ? AND visibility = 'visible'`,
		guestID, time.Now().UnixMilli(), mediaID)
	if err != nil {
		return fmt.Errorf("insert like: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count inserted likes: %w", err)
	}
	if inserted == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE media SET like_count = like_count + 1 WHERE id = ?", mediaID); err != nil {
		return fmt.Errorf("increment like count: %w", err)
	}
	return nil
}

// deleteLike removes the row and decrements the aggregate only when a visible
// medium actually lost a like.
func deleteLike(ctx context.Context, tx *sql.Tx, guestID, mediaID string) error {
	result, err := tx.ExecContext(ctx,
		`DELETE FROM media_likes WHERE media_id = ? AND guest_id = ?
		   AND EXISTS (SELECT 1 FROM media WHERE id = ? AND visibility = 'visible')`,
		mediaID, guestID, mediaID)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted likes: %w", err)
	}
	if removed == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE media SET like_count = like_count - 1 WHERE id = ?", mediaID); err != nil {
		return fmt.Errorf("decrement like count: %w", err)
	}
	return nil
}
