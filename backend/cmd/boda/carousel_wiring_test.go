package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/media"
)

func TestAppHandlerWiresCarouselFromConfiguredPeriods(t *testing.T) {
	configureTestEnvironment(t, t.TempDir())
	t.Setenv("BODA_PERIOD_1_START", "2026-10-18T18:00")
	t.Setenv("BODA_PERIOD_1_END", "2026-10-18T20:00")
	t.Setenv("BODA_CAROUSEL_SIZE", "3")
	// The default free-disk floor (15 GB) would refuse the test photo on a small disk.
	t.Setenv("BODA_DISK_MIN_FREE_BYTES", "1")
	ctx := context.Background()
	database, cfg, paths, err := initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	photos, err := newPhotoStore(database, cfg, paths)
	if err != nil {
		t.Fatal(err)
	}
	gallery, err := media.NewGallery(database, paths)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAppHandler(ctx, database, cfg, photos, gallery, paths)
	if err != nil {
		t.Fatalf("newAppHandler() = %v", err)
	}

	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	const id = "123e4567-e89b-42d3-a456-426614174000"
	guest := auth.NewGuestCookie([]byte(cfg.SessionKey), time.Now())
	metadata := "id " + base64.StdEncoding.EncodeToString([]byte(id)) + ",captured_at " + base64.StdEncoding.EncodeToString([]byte("2026-10-18T18:30:00"))
	for _, step := range []struct {
		method, path, offset string
		body                 []byte
		want                 int
	}{
		{http.MethodPost, "/api/uploads/", "", nil, http.StatusCreated},
		{http.MethodPatch, "/api/uploads/" + id, "0", jpeg, http.StatusNoContent},
	} {
		request := httptest.NewRequest(step.method, step.path, bytes.NewReader(step.body))
		request.Header.Set("Tus-Resumable", "1.0.0")
		request.AddCookie(guest)
		if step.method == http.MethodPost {
			request.Header.Set("Upload-Length", strconv.Itoa(len(jpeg)))
			request.Header.Set("Upload-Metadata", metadata)
		} else {
			request.Header.Set("Upload-Offset", step.offset)
			request.Header.Set("Content-Type", "application/offset+octet-stream")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != step.want {
			t.Fatalf("%s %s = %d %q; want %d", step.method, step.path, response.Code, response.Body.String(), step.want)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/carousels", nil)
	request.AddCookie(guest)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var strip struct {
		Periods []struct {
			ID    int    `json:"id"`
			Total int64  `json:"total"`
			Start string `json:"start"`
		} `json:"periods"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &strip); err != nil || response.Code != http.StatusOK {
		t.Fatalf("GET /api/carousels = %d %q (%v); want 200 JSON", response.Code, response.Body.String(), err)
	}
	if len(strip.Periods) != 1 || strip.Periods[0].ID != 1 || strip.Periods[0].Total != 1 || strip.Periods[0].Start != "2026-10-18T18:00" {
		t.Fatalf("strip periods = %+v; want period 1 from configuration with the stored photo", strip.Periods)
	}

	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/carousels/1", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/carousels/1 without session = %d; want 401", anonymous.Code)
	}
}

func TestLoadRejectsPeriodGapAtStartup(t *testing.T) {
	configureTestEnvironment(t, t.TempDir())
	t.Setenv("BODA_PERIOD_1_START", "2026-10-18T18:00")
	t.Setenv("BODA_PERIOD_1_END", "2026-10-18T19:00")
	t.Setenv("BODA_PERIOD_3_START", "2026-10-18T20:00")
	t.Setenv("BODA_PERIOD_3_END", "2026-10-18T21:00")
	if _, _, _, err := initialize(context.Background()); err == nil {
		t.Fatal("initialize() with a period gap succeeded; want startup failure")
	}
}
