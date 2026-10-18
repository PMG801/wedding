package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PMG801/wedding/internal/upload"
)

// ErrPreviewNotFound reports that no private preview exists for that request.
var ErrPreviewNotFound = errors.New("hidden preview not found")

// ListHidden pages hidden media for the administrator, newest first, with the
// same cursor as List. Hidden files are never served by the proxy, so the
// original URL is nil and previews point at the admin preview route.
func (g *Gallery) ListHidden(ctx context.Context, cursor string, limit int) (Page, error) {
	return g.list(ctx, "hidden", "", cursor, limit, func(item *Item, _ string) error {
		var err error
		if item.ThumbURL, err = g.previewURL(item.ID, "t"); err != nil {
			return err
		}
		item.DetailURL, err = g.previewURL(item.ID, "d")
		return err
	})
}

func (g *Gallery) previewURL(id, variant string) (*string, error) {
	if _, err := os.Stat(g.stagedPath(id, variant)); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect hidden preview: %w", err)
	}
	url := "/api/admin/media/" + id + "/preview/" + variant
	return &url, nil
}

// OpenHiddenPreview opens the private derivative of one hidden medium.
func (g *Gallery) OpenHiddenPreview(ctx context.Context, id, variant string) (*os.File, error) {
	if !upload.ValidID(id) || (variant != "t" && variant != "d") {
		return nil, ErrPreviewNotFound
	}
	var visibility string
	err := g.db.QueryRowContext(ctx, "SELECT visibility FROM media WHERE id = ?", id).Scan(&visibility)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && visibility != "hidden") {
		return nil, ErrPreviewNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read media visibility: %w", err)
	}
	file, err := os.Open(g.stagedPath(id, variant))
	if os.IsNotExist(err) {
		return nil, ErrPreviewNotFound
	}
	return file, err
}

func (g *Gallery) stagedPath(id, variant string) string {
	return filepath.Join(g.tmpDir, id+"_"+variant+".staged")
}
