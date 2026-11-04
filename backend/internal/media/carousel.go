package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PMG801/wedding/internal/upload"
)

// ErrPeriodNotFound reports an album request for a period that is not configured.
var ErrPeriodNotFound = errors.New("carousel period not found")

// Period is one configured carousel window (ADR-0015). Start and End are
// YYYY-MM-DDTHH:MM in event local time; the window is half-open: [Start, End).
type Period struct {
	ID    int
	Start string
	End   string
}

// CarouselConfig holds the carousel settings. Now is the injectable clock and
// defaults to time.Now.
type CarouselConfig struct {
	Periods  []Period
	Size     int
	Rotation time.Duration
	Now      func() time.Time
}

// Carousel serves the story strip (one bubble per period with content) and the
// full album of each period. Visible media only, like the gallery.
type Carousel struct {
	gallery  *Gallery
	periods  []Period
	size     int
	rotation time.Duration
	now      func() time.Time
}

// Bubble is one period in the strip: at most size items for the current slot,
// and Total, the number of visible media in the window.
type Bubble struct {
	ID    int    `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
	Total int64  `json:"total"`
	Items []Item `json:"items"`
}

// Strip is the story strip. RotatesAt is the start of the next slot in Unix ms.
type Strip struct {
	RotatesAt int64    `json:"rotates_at"`
	Periods   []Bubble `json:"periods"`
}

// carouselRow is a visible medium of a period, with its capture time for keyset paging.
type carouselRow struct {
	item        Item
	storageName string
	capturedAt  string
}

// NewCarousel builds the carousel reader. A nil gallery or a non-positive size
// or rotation is a programming error, not a configuration choice.
func NewCarousel(gallery *Gallery, cfg CarouselConfig) (*Carousel, error) {
	if gallery == nil {
		return nil, errors.New("carousel requires a gallery")
	}
	if cfg.Size < 1 {
		return nil, errors.New("carousel size must be positive")
	}
	if cfg.Rotation <= 0 {
		return nil, errors.New("carousel rotation must be positive")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Carousel{gallery: gallery, periods: cfg.Periods, size: cfg.Size, rotation: cfg.Rotation, now: now}, nil
}

// Strip returns one bubble per configured period that has visible media. Each
// bubble holds a per-slot selection that is identical for every guest, in capture order.
func (c *Carousel) Strip(ctx context.Context, guestID string) (Strip, error) {
	slot, rotatesAt := c.slotAt(c.now())
	strip := Strip{RotatesAt: rotatesAt, Periods: []Bubble{}}
	for _, period := range c.periods {
		rows, err := c.windowRows(ctx, guestID, period)
		if err != nil {
			return Strip{}, err
		}
		if len(rows) == 0 {
			continue
		}
		bubble := Bubble{ID: period.ID, Start: period.Start, End: period.End, Total: int64(len(rows)), Items: []Item{}}
		chosen := pickSlot(rows, period.ID, slot, c.size)
		items, err := c.itemsFor(rows, chosen)
		if err != nil {
			return Strip{}, err
		}
		bubble.Items = items
		strip.Periods = append(strip.Periods, bubble)
	}
	return strip, nil
}

// Album pages every visible medium of a period by capture time, ascending.
// The cursor is the last (captured_at, id) pair of the previous page.
func (c *Carousel) Album(ctx context.Context, guestID string, periodID int, cursor string, limit int) (Page, error) {
	period, ok := c.period(periodID)
	if !ok {
		return Page{}, ErrPeriodNotFound
	}
	if limit <= 0 {
		return Page{}, errors.New("album limit must be positive")
	}
	var cursorCaptured, cursorID string
	if cursor != "" {
		var err error
		if cursorCaptured, cursorID, err = decodeCaptureCursor(cursor); err != nil {
			return Page{}, err
		}
	}
	rows, err := c.db().QueryContext(ctx, albumSQL(cursor != ""), albumArgs(guestID, period, cursorCaptured, cursorID, limit)...)
	if err != nil {
		return Page{}, fmt.Errorf("query album: %w", err)
	}
	defer func() { _ = rows.Close() }()
	scanned, err := scanCarouselRows(rows)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: []Item{}}
	if len(scanned) > limit {
		scanned = scanned[:limit]
		last := scanned[limit-1]
		next := encodeCaptureCursor(last.capturedAt, last.item.ID)
		page.NextCursor = &next
	}
	chosen := make([]int, len(scanned))
	for i := range chosen {
		chosen[i] = i
	}
	page.Items, err = c.itemsFor(scanned, chosen)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

func (c *Carousel) db() *sql.DB { return c.gallery.db }

func (c *Carousel) period(id int) (Period, bool) {
	for _, period := range c.periods {
		if period.ID == id {
			return period, true
		}
	}
	return Period{}, false
}

// slotAt returns the rotation slot index of now and the Unix ms at which it ends.
func (c *Carousel) slotAt(now time.Time) (slot int64, rotatesAt int64) {
	rotation := c.rotation.Nanoseconds()
	slot = now.UnixNano() / rotation
	rotatesAt = time.Unix(0, (slot+1)*rotation).UnixMilli()
	return slot, rotatesAt
}

// windowRows loads every visible medium of the period in capture order.
func (c *Carousel) windowRows(ctx context.Context, guestID string, period Period) ([]carouselRow, error) {
	rows, err := c.db().QueryContext(ctx, windowSQL, guestID, windowStart(period), windowEnd(period))
	if err != nil {
		return nil, fmt.Errorf("query carousel period: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCarouselRows(rows)
}

// itemsFor fills URLs for the chosen rows, keeping their order. chosen holds
// ascending indexes into rows.
func (c *Carousel) itemsFor(rows []carouselRow, chosen []int) ([]Item, error) {
	items := make([]Item, 0, len(chosen))
	for _, index := range chosen {
		item := rows[index].item
		if err := c.gallery.visibleURLs(&item, rows[index].storageName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// pickSlot returns the ascending indexes of the size rows that rank first for
// (period, slot). Ranking by a hash of the seed is a deterministic shuffle: the
// same seed gives the same choice for every guest, and a new slot gives a new one.
func pickSlot(rows []carouselRow, periodID int, slot int64, size int) []int {
	if len(rows) <= size {
		all := make([]int, len(rows))
		for i := range all {
			all[i] = i
		}
		return all
	}
	type ranked struct {
		index int
		key   [sha256.Size]byte
	}
	order := make([]ranked, len(rows))
	for i, row := range rows {
		order[i] = ranked{index: i, key: sha256.Sum256([]byte(fmt.Sprintf("boda-carousel|%d|%d|%s", periodID, slot, row.item.ID)))}
	}
	sort.Slice(order, func(a, b int) bool {
		if cmp := bytes.Compare(order[a].key[:], order[b].key[:]); cmp != 0 {
			return cmp < 0
		}
		return order[a].index < order[b].index
	})
	chosen := make([]int, size)
	for i := range chosen {
		chosen[i] = order[i].index
	}
	sort.Ints(chosen)
	return chosen
}

// carouselSelect is the gallery item columns plus the capture time. The first
// placeholder is the guest identity for liked_by_me.
const carouselSelect = "SELECT m.id, m.storage_name, m.media_type, m.confirmed_at, m.like_count, " + likedByGuest + ", m.captured_at" +
	" FROM media m WHERE m.visibility = 'visible' AND m.captured_at IS NOT NULL AND m.captured_at >= ? AND m.captured_at < ?"

// windowSQL lists every visible medium of a window in capture order.
const windowSQL = carouselSelect + " ORDER BY m.captured_at ASC, m.id ASC"

// albumSQL pages a window by the keyset (captured_at, id), ascending. Its WHERE
// and ORDER BY match the partial index media_visible_captured.
func albumSQL(withCursor bool) string {
	query := carouselSelect
	if withCursor {
		query += " AND (m.captured_at > ? OR (m.captured_at = ? AND m.id > ?))"
	}
	return query + " ORDER BY m.captured_at ASC, m.id ASC LIMIT ?"
}

// albumArgs binds albumSQL: guest, window bounds, optional cursor, then limit+1
// so the caller can tell whether a next page exists.
func albumArgs(guestID string, period Period, cursorCaptured, cursorID string, limit int) []any {
	args := []any{guestID, windowStart(period), windowEnd(period)}
	if cursorCaptured != "" {
		args = append(args, cursorCaptured, cursorCaptured, cursorID)
	}
	return append(args, limit+1)
}

// windowStart and windowEnd turn YYYY-MM-DDTHH:MM into the second-precision
// bounds that compare with captured_at as strings.
func windowStart(period Period) string { return period.Start + ":00" }
func windowEnd(period Period) string   { return period.End + ":00" }

func scanCarouselRows(rows *sql.Rows) ([]carouselRow, error) {
	result := []carouselRow{}
	for rows.Next() {
		var row carouselRow
		if err := rows.Scan(&row.item.ID, &row.storageName, &row.item.MIME, &row.item.ConfirmedAt, &row.item.LikeCount, &row.item.LikedByMe, &row.capturedAt); err != nil {
			return nil, fmt.Errorf("read carousel row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read carousel rows: %w", err)
	}
	return result, nil
}

// encodeCaptureCursor and decodeCaptureCursor carry the album keyset. The
// capture string has colons, so the separator is '|'.
func encodeCaptureCursor(capturedAt, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(capturedAt + "|" + id))
}

func decodeCaptureCursor(cursor string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", ErrInvalidCursor
	}
	capturedAt, id, found := strings.Cut(string(raw), "|")
	if !found || !upload.ValidID(id) {
		return "", "", ErrInvalidCursor
	}
	if _, ok := upload.NormalizeCapturedAt(capturedAt); !ok {
		return "", "", ErrInvalidCursor
	}
	return capturedAt, id, nil
}
