// Package pinned stores the administrator's pinned text entries (ADR-0013).
package pinned

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
)

// MaxItems is the most pinned entries the gallery may hold (ADR-0013, D3).
const MaxItems = 5

const (
	maxTitleRunes = 120
	maxBodyRunes  = 2000
	// tempPositionBase lifts rows out of 0..MaxItems-1 while positions are
	// rewritten, so no two rows ever share a position inside the UNIQUE index.
	tempPositionBase = 1 << 20
)

var (
	// ErrInvalid reports text or a media reference that breaks the rules of D7.
	ErrInvalid = errors.New("invalid pinned item")
	// ErrNotFound reports that no entry has the requested identifier.
	ErrNotFound = errors.New("pinned item not found")
	// ErrLimitReached reports that MaxItems entries already exist; nothing is stored.
	ErrLimitReached = errors.New("pinned item limit reached")
	// ErrInvalidOrder reports ids that are not exactly the current set.
	ErrInvalidOrder = errors.New("invalid pinned order")
)

// Input is the editable content of one entry. MediaID nil means no image.
type Input struct {
	Title   string
	Body    string
	MediaID *string
}

// Image is the resolved picture of an entry. It exists only while the
// referenced medium is visible and has a published thumbnail.
type Image struct {
	ID        string
	ThumbURL  string
	DetailURL *string
}

// Item is one stored entry with its resolved image.
type Item struct {
	ID        string
	Title     string
	Body      string
	MediaID   *string
	Position  int
	CreatedAt int64
	UpdatedAt int64
	Image     *Image
}

// Store reads and writes pinned entries in SQLite.
type Store struct {
	db         *sql.DB
	derivedDir string
}

// New builds the store over an initialized database and storage paths.
func New(database *sql.DB, paths storage.Paths) (*Store, error) {
	if database == nil || paths.Derived == "" {
		return nil, errors.New("database and initialized storage paths are required")
	}
	return &Store{db: database, derivedDir: paths.Derived}, nil
}

// List returns every entry in manual order.
func (s *Store) List(ctx context.Context) ([]Item, error) {
	return s.query(ctx, s.db, "")
}

// Create appends an entry at the end of the order. The limit check and the
// insert are one statement that takes the write lock first, so concurrent
// creates cannot exceed MaxItems.
func (s *Store) Create(ctx context.Context, in Input) (Item, error) {
	input, err := normalize(in)
	if err != nil {
		return Item{}, err
	}
	id, err := newID()
	if err != nil {
		return Item{}, err
	}
	now := time.Now().UnixMilli()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, fmt.Errorf("begin pinned create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO pinned_items (id, title, body, media_id, position, created_at, updated_at)
		 SELECT ?, ?, ?, ?, coalesce((SELECT max(position) + 1 FROM pinned_items), 0), ?, ?
		  WHERE (SELECT count(*) FROM pinned_items) < ?
		    AND (? IS NULL OR EXISTS (SELECT 1 FROM media WHERE id = ?))`,
		id, input.Title, input.Body, input.MediaID, now, now, MaxItems, input.MediaID, input.MediaID)
	if err != nil {
		return Item{}, fmt.Errorf("insert pinned item: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Item{}, fmt.Errorf("count inserted pinned items: %w", err)
	}
	if inserted == 0 {
		// Nothing was written; report why. Invalid media wins over the limit.
		if input.MediaID != nil {
			exists, err := rowCount(ctx, tx, "SELECT count(*) FROM media WHERE id = ?", *input.MediaID)
			if err != nil {
				return Item{}, err
			}
			if exists == 0 {
				return Item{}, ErrInvalid
			}
		}
		return Item{}, ErrLimitReached
	}

	items, err := s.query(ctx, tx, " WHERE p.id = ?", id)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, fmt.Errorf("commit pinned create: %w", err)
	}
	if len(items) != 1 {
		return Item{}, fmt.Errorf("created pinned item %s not readable", id)
	}
	return items[0], nil
}

// Update replaces the content of an entry and keeps its position. The UPDATE
// runs first so the transaction holds the write lock before any check.
func (s *Store) Update(ctx context.Context, id string, in Input) (Item, error) {
	input, err := normalize(in)
	if err != nil {
		return Item{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, fmt.Errorf("begin pinned update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`UPDATE pinned_items SET title = ?, body = ?, media_id = ?, updated_at = ?
		  WHERE id = ? AND (? IS NULL OR EXISTS (SELECT 1 FROM media WHERE id = ?))`,
		input.Title, input.Body, input.MediaID, time.Now().UnixMilli(), id, input.MediaID, input.MediaID)
	if err != nil {
		return Item{}, fmt.Errorf("update pinned item: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Item{}, fmt.Errorf("count updated pinned items: %w", err)
	}
	if updated == 0 {
		found, err := rowCount(ctx, tx, "SELECT count(*) FROM pinned_items WHERE id = ?", id)
		if err != nil {
			return Item{}, err
		}
		if found == 0 {
			return Item{}, ErrNotFound
		}
		return Item{}, ErrInvalid
	}

	items, err := s.query(ctx, tx, " WHERE p.id = ?", id)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, fmt.Errorf("commit pinned update: %w", err)
	}
	if len(items) != 1 {
		return Item{}, ErrNotFound
	}
	return items[0], nil
}

// Delete removes an entry and compacts the positions of the remaining ones.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pinned delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, "DELETE FROM pinned_items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete pinned item: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted pinned items: %w", err)
	}
	if removed == 0 {
		return ErrNotFound
	}
	remaining, err := orderedIDs(ctx, tx)
	if err != nil {
		return err
	}
	if err := moveToTemporary(ctx, tx, remaining); err != nil {
		return err
	}
	if err := setFinal(ctx, tx, remaining); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pinned delete: %w", err)
	}
	return nil
}

// Reorder sets the order to ids, which must be exactly the current entries
// without duplicates. A rejected list leaves the stored order untouched.
func (s *Store) Reorder(ctx context.Context, ids []string) ([]Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pinned reorder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Temporary positions come first so the transaction takes the write lock.
	// A rejected list rolls them back together with everything else.
	if err := moveToTemporary(ctx, tx, ids); err != nil {
		return nil, err
	}
	current, err := orderedIDs(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !sameSet(ids, current) {
		return nil, ErrInvalidOrder
	}
	if err := setFinal(ctx, tx, ids); err != nil {
		return nil, err
	}
	items, err := s.query(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pinned reorder: %w", err)
	}
	return items, nil
}

// normalize applies the D7 rules and returns the value that will be stored.
func normalize(in Input) (Input, error) {
	title := strings.TrimSpace(in.Title)
	if !validText(title, 1, maxTitleRunes, false) || !validText(in.Body, 0, maxBodyRunes, true) {
		return Input{}, ErrInvalid
	}
	if in.MediaID != nil && !upload.ValidID(*in.MediaID) {
		return Input{}, ErrInvalid
	}
	return Input{Title: title, Body: in.Body, MediaID: in.MediaID}, nil
}

// validText checks UTF-8, the length in code points and control characters.
// Line feeds are allowed only when multiline is set.
func validText(s string, minRunes, maxRunes int, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	length := utf8.RuneCountInString(s)
	if length < minRunes || length > maxRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return false
		}
	}
	return true
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

const selectItems = `SELECT p.id, p.title, p.body, p.media_id, p.position, p.created_at, p.updated_at, m.visibility
  FROM pinned_items p LEFT JOIN media m ON m.id = p.media_id`

// query reads entries in position order; where is appended before ORDER BY.
func (s *Store) query(ctx context.Context, q querier, where string, args ...any) ([]Item, error) {
	rows, err := q.QueryContext(ctx, selectItems+where+" ORDER BY p.position", args...)
	if err != nil {
		return nil, fmt.Errorf("query pinned items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []Item{}
	var visibilities []sql.NullString
	for rows.Next() {
		var item Item
		var mediaID, visibility sql.NullString
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &mediaID, &item.Position, &item.CreatedAt, &item.UpdatedAt, &visibility); err != nil {
			return nil, fmt.Errorf("read pinned row: %w", err)
		}
		if mediaID.Valid {
			id := mediaID.String
			item.MediaID = &id
		}
		items = append(items, item)
		visibilities = append(visibilities, visibility)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pinned rows: %w", err)
	}
	for i := range items {
		image, err := s.resolveImage(items[i].MediaID, visibilities[i])
		if err != nil {
			return nil, err
		}
		items[i].Image = image
	}
	return items, nil
}

// resolveImage returns the image only for a visible medium with a published
// thumbnail. The disk decides whether the derivative exists.
func (s *Store) resolveImage(mediaID *string, visibility sql.NullString) (*Image, error) {
	if mediaID == nil || !visibility.Valid || visibility.String != "visible" {
		return nil, nil
	}
	hasThumb, err := s.published(*mediaID, "t")
	if err != nil || !hasThumb {
		return nil, err
	}
	image := &Image{ID: *mediaID, ThumbURL: "/m/d/" + upload.DerivativeFileName(*mediaID, "t")}
	hasDetail, err := s.published(*mediaID, "d")
	if err != nil {
		return nil, err
	}
	if hasDetail {
		detail := "/m/d/" + upload.DerivativeFileName(*mediaID, "d")
		image.DetailURL = &detail
	}
	return image, nil
}

func (s *Store) published(mediaID, variant string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.derivedDir, upload.DerivativeFileName(mediaID, variant)))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("inspect published derivative: %w", err)
}

func rowCount(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return count, nil
}

func orderedIDs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM pinned_items ORDER BY position")
	if err != nil {
		return nil, fmt.Errorf("list pinned ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read pinned id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pinned ids: %w", err)
	}
	return ids, nil
}

// moveToTemporary parks each listed row above the normal range.
func moveToTemporary(ctx context.Context, tx *sql.Tx, ids []string) error {
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE pinned_items SET position = ? WHERE id = ?", tempPositionBase+i, id); err != nil {
			return fmt.Errorf("park pinned position: %w", err)
		}
	}
	return nil
}

// setFinal assigns 0, 1, 2… in the order of ids. Callers pass every row, and
// all of them are parked, so the final values cannot collide.
func setFinal(ctx context.Context, tx *sql.Tx, ids []string) error {
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE pinned_items SET position = ? WHERE id = ?", i, id); err != nil {
			return fmt.Errorf("set pinned position: %w", err)
		}
	}
	return nil
}

// sameSet reports whether ids holds each of current exactly once and nothing else.
func sameSet(ids, current []string) bool {
	if len(ids) != len(current) {
		return false
	}
	want := make(map[string]struct{}, len(current))
	for _, id := range current {
		want[id] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := want[id]; !ok {
			return false
		}
		delete(want, id)
	}
	return true
}

// newID returns a random UUIDv4 in canonical lowercase form.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate pinned id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
