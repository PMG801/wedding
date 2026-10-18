// Package media lists confirmed, visible gallery media.
package media

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

// ErrInvalidCursor reports a cursor that this server did not issue.
var ErrInvalidCursor = errors.New("invalid gallery cursor")

// topLimit caps the popularity ranking; it is one response without a cursor.
const topLimit = 50

// likedByGuest is 1 when the guest identity has a like row for the medium.
const likedByGuest = "EXISTS (SELECT 1 FROM media_likes l WHERE l.media_id = m.id AND l.guest_id = ?)"

// topQuery lists visible media with at least one like, most liked first. Its
// WHERE and ORDER BY match the partial index media_visible_popularity.
const topQuery = "SELECT m.id, m.storage_name, m.media_type, m.confirmed_at, m.like_count, " + likedByGuest +
	" FROM media m WHERE m.visibility = 'visible' AND m.like_count > 0" +
	" ORDER BY m.like_count DESC, m.confirmed_at DESC, m.id DESC LIMIT ?"

// Item is one visible, confirmed medium. Derivative URLs are nil until the
// corresponding JPEG has been published. LikeCount is the stored number of
// «me gusta»; LikedByMe tells whether the requesting guest identity marked it.
type Item struct {
	ID          string  `json:"id"`
	MIME        string  `json:"mime"`
	ConfirmedAt int64   `json:"confirmed_at"`
	OriginalURL *string `json:"original_url"`
	ThumbURL    *string `json:"thumb_url"`
	DetailURL   *string `json:"detail_url"`
	LikeCount   int64   `json:"like_count"`
	LikedByMe   bool    `json:"liked_by_me"`
}

// Page is one newest-first gallery page. NextCursor is nil on the last page.
type Page struct {
	Items      []Item  `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type Gallery struct {
	db         *sql.DB
	derivedDir string
	tmpDir     string
}

// NewGallery builds a gallery reader over initialized SQLite and storage paths.
func NewGallery(database *sql.DB, paths storage.Paths) (*Gallery, error) {
	if database == nil || paths.Derived == "" {
		return nil, errors.New("database and initialized storage paths are required")
	}
	return &Gallery{db: database, derivedDir: paths.Derived, tmpDir: paths.Tmp}, nil
}

// List returns the chronological page for an anonymous viewer: LikedByMe is
// always false. Guests use ListForGuest.
func (g *Gallery) List(ctx context.Context, cursor string, limit int) (Page, error) {
	return g.ListForGuest(ctx, "", cursor, limit)
}

// ListForGuest returns up to limit visible media older than cursor, newest
// first, with the like state for guestID. Keyset pagination on (confirmed_at,
// id) keeps pages stable while new uploads arrive.
func (g *Gallery) ListForGuest(ctx context.Context, guestID, cursor string, limit int) (Page, error) {
	return g.list(ctx, "visible", guestID, cursor, limit, g.visibleURLs)
}

// Top returns the most liked visible media, at most 50 and without a cursor.
// Only media with at least one like are included.
func (g *Gallery) Top(ctx context.Context, guestID string) ([]Item, error) {
	rows, err := g.db.QueryContext(ctx, topQuery, guestID, topLimit)
	if err != nil {
		return nil, fmt.Errorf("query top gallery: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items, storageNames, err := scanItems(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if err := g.visibleURLs(&items[i], storageNames[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// visibleURLs fills the original and derivative URLs of a visible medium.
func (g *Gallery) visibleURLs(item *Item, storageName string) error {
	original := "/m/o/" + storageName
	item.OriginalURL = &original
	var err error
	if item.ThumbURL, err = g.derivativeURL(item.ID, "t"); err != nil {
		return err
	}
	item.DetailURL, err = g.derivativeURL(item.ID, "d")
	return err
}

// list pages media of one visibility; urls fills each item's URLs. guestID
// selects which like state is reported; an empty guestID reports none.
func (g *Gallery) list(ctx context.Context, visibility, guestID, cursor string, limit int, urls func(*Item, string) error) (Page, error) {
	if limit <= 0 {
		return Page{}, errors.New("gallery limit must be positive")
	}
	query := "SELECT m.id, m.storage_name, m.media_type, m.confirmed_at, m.like_count, " + likedByGuest +
		" FROM media m WHERE m.visibility = ?"
	args := []any{guestID, visibility}
	if cursor != "" {
		confirmedAt, id, err := decodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		query += " AND (m.confirmed_at < ? OR (m.confirmed_at = ? AND m.id < ?))"
		args = append(args, confirmedAt, confirmedAt, id)
	}
	query += " ORDER BY m.confirmed_at DESC, m.id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := g.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Page{}, fmt.Errorf("query gallery: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, storageNames, err := scanItems(rows)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		storageNames = storageNames[:limit]
		last := page.Items[limit-1]
		next := encodeCursor(last.ConfirmedAt, last.ID)
		page.NextCursor = &next
	}
	for i := range page.Items {
		if err := urls(&page.Items[i], storageNames[i]); err != nil {
			return Page{}, err
		}
	}
	return page, nil
}

// scanItems reads rows selected as id, storage_name, media_type,
// confirmed_at, like_count, liked_by_me. It never returns a nil slice.
func scanItems(rows *sql.Rows) ([]Item, []string, error) {
	items := []Item{}
	var storageNames []string
	for rows.Next() {
		var item Item
		var storageName string
		if err := rows.Scan(&item.ID, &storageName, &item.MIME, &item.ConfirmedAt, &item.LikeCount, &item.LikedByMe); err != nil {
			return nil, nil, fmt.Errorf("read gallery row: %w", err)
		}
		items = append(items, item)
		storageNames = append(storageNames, storageName)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read gallery rows: %w", err)
	}
	return items, storageNames, nil
}

// derivativeURL checks the published file on disk: the disk decides whether a
// derivative exists.
func (g *Gallery) derivativeURL(id, variant string) (*string, error) {
	name := upload.DerivativeFileName(id, variant)
	if _, err := os.Stat(filepath.Join(g.derivedDir, name)); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect published derivative: %w", err)
	}
	url := "/m/d/" + name
	return &url, nil
}

func encodeCursor(confirmedAt int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(confirmedAt, 10) + ":" + id))
}

func decodeCursor(cursor string) (int64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", ErrInvalidCursor
	}
	timestamp, id, found := strings.Cut(string(raw), ":")
	if !found || !upload.ValidID(id) {
		return 0, "", ErrInvalidCursor
	}
	confirmedAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return 0, "", ErrInvalidCursor
	}
	return confirmedAt, id, nil
}
