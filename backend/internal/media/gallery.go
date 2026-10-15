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

// Item is one visible, confirmed medium. Derivative URLs are nil until the
// corresponding JPEG has been published.
type Item struct {
	ID          string  `json:"id"`
	MIME        string  `json:"mime"`
	ConfirmedAt int64   `json:"confirmed_at"`
	OriginalURL string  `json:"original_url"`
	ThumbURL    *string `json:"thumb_url"`
	DetailURL   *string `json:"detail_url"`
}

// Page is one newest-first gallery page. NextCursor is nil on the last page.
type Page struct {
	Items      []Item  `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type Gallery struct {
	db         *sql.DB
	derivedDir string
}

// NewGallery builds a gallery reader over initialized SQLite and storage paths.
func NewGallery(database *sql.DB, paths storage.Paths) (*Gallery, error) {
	if database == nil || paths.Derived == "" {
		return nil, errors.New("database and initialized storage paths are required")
	}
	return &Gallery{db: database, derivedDir: paths.Derived}, nil
}

// List returns up to limit visible media older than cursor, newest first.
// Keyset pagination on (confirmed_at, id) keeps pages stable while new
// uploads arrive.
func (g *Gallery) List(ctx context.Context, cursor string, limit int) (Page, error) {
	if limit <= 0 {
		return Page{}, errors.New("gallery limit must be positive")
	}
	query := "SELECT id, storage_name, media_type, confirmed_at FROM media WHERE visibility = 'visible'"
	args := []any{}
	if cursor != "" {
		confirmedAt, id, err := decodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		query += " AND (confirmed_at < ? OR (confirmed_at = ? AND id < ?))"
		args = append(args, confirmedAt, confirmedAt, id)
	}
	query += " ORDER BY confirmed_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := g.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Page{}, fmt.Errorf("query gallery: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := Page{Items: []Item{}}
	for rows.Next() {
		var item Item
		var storageName string
		if err := rows.Scan(&item.ID, &storageName, &item.MIME, &item.ConfirmedAt); err != nil {
			return Page{}, fmt.Errorf("read gallery row: %w", err)
		}
		item.OriginalURL = "/m/o/" + storageName
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("read gallery rows: %w", err)
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		next := encodeCursor(last.ConfirmedAt, last.ID)
		page.NextCursor = &next
	}
	for i := range page.Items {
		if page.Items[i].ThumbURL, err = g.derivativeURL(page.Items[i].ID, "t"); err != nil {
			return Page{}, err
		}
		if page.Items[i].DetailURL, err = g.derivativeURL(page.Items[i].ID, "d"); err != nil {
			return Page{}, err
		}
	}
	return page, nil
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
