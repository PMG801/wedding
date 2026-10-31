package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/PMG801/wedding/internal/upload"
)

// capturedMetadata is the creation metadata of id with an optional captured_at
// entry (ADR-0015): present=false leaves the key out.
func capturedMetadata(id, capturedAt string, present bool) string {
	metadata := idMetadata(id)
	if present {
		metadata += ",captured_at " + base64.StdEncoding.EncodeToString([]byte(capturedAt))
	}
	return metadata
}

// uploadCaptured creates and completes the tus upload of body with the given metadata.
func (f *resumableFixture) uploadCaptured(id, metadata string, body []byte) {
	f.t.Helper()
	created := f.serve(tusCreate(metadata, int64(len(body)), f.cookie))
	if created.Code != http.StatusCreated {
		f.t.Fatalf("create upload %s = %d %q; want 201", id, created.Code, created.Body.String())
	}
	patched := f.serve(tusPatch(id, 0, bytes.NewReader(body), int64(len(body)), f.cookie))
	if patched.Code != http.StatusNoContent {
		f.t.Fatalf("PATCH upload %s = %d %q; want 204", id, patched.Code, patched.Body.String())
	}
}

func (f *resumableFixture) storedCaptured(id string) (string, bool) {
	f.t.Helper()
	var captured sql.NullString
	if err := f.database.QueryRow("SELECT captured_at FROM media WHERE id = ?", id).Scan(&captured); err != nil {
		f.t.Fatal(err)
	}
	return captured.String, captured.Valid
}

func TestResumableStoresValidCapturedAtMetadata(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.uploadCaptured(tusIDOne, capturedMetadata(tusIDOne, "2026-10-18T18:05:09", true), jpegBody(64))
	if got, ok := f.storedCaptured(tusIDOne); !ok || got != "2026-10-18T18:05:09" {
		t.Fatalf("stored captured_at = %q, %v; want 2026-10-18T18:05:09", got, ok)
	}
}

func TestResumableStoresCapturedAtOfVideo(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxVideoBytes: 4096, MaxConcurrent: 2}, 2)
	mp4 := append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 52)...)
	f.uploadCaptured(tusIDOne, capturedMetadata(tusIDOne, "2026-10-18T21:30:00", true), mp4)
	var mime string
	if err := f.database.QueryRow("SELECT media_type FROM media WHERE id = ?", tusIDOne).Scan(&mime); err != nil || mime != "video/mp4" {
		t.Fatalf("stored media_type = %q (%v); want video/mp4", mime, err)
	}
	if got, ok := f.storedCaptured(tusIDOne); !ok || got != "2026-10-18T21:30:00" {
		t.Fatalf("stored captured_at = %q, %v; want 2026-10-18T21:30:00", got, ok)
	}
}

func TestResumableIgnoresMissingOrInvalidCapturedAtAndStillConfirms(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		present bool
	}{
		{"missing key", "", false},
		{"empty value", "", true},
		{"wrong separator", "2026-10-18 18:05:09", true},
		{"impossible date", "2026-02-30T10:00:00", true},
		{"year before 2000", "1999-12-31T23:59:59", true},
		{"year after 2100", "2101-01-01T00:00:00", true},
		{"zone suffix", "2026-10-18T18:05:09Z", true},
		{"unix milliseconds", "1790000000000", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
			f.uploadCaptured(tusIDOne, capturedMetadata(tusIDOne, test.value, test.present), jpegBody(64))
			if rows := f.mediaRows(); rows != 1 {
				t.Fatalf("media rows = %d; want 1", rows)
			}
			if got, ok := f.storedCaptured(tusIDOne); ok {
				t.Fatalf("stored captured_at = %q; want NULL", got)
			}
		})
	}
}

func TestResumableRetryNeverChangesStoredCapturedAt(t *testing.T) {
	f := newResumableFixture(t, upload.Options{MaxBytes: 1024, MaxConcurrent: 2}, 2)
	f.uploadCaptured(tusIDOne, capturedMetadata(tusIDOne, "2026-10-18T18:05:09", true), jpegBody(64))
	retry := f.serve(tusCreate(capturedMetadata(tusIDOne, "2020-01-01T00:00:00", true), 64, f.cookie))
	if retry.Code != http.StatusConflict || retry.Body.String() != "{\"error\":\"already_confirmed\"}\n" {
		t.Fatalf("retry create = %d %q; want 409 already_confirmed", retry.Code, retry.Body.String())
	}
	head := f.serve(tusHead(tusIDOne, f.cookie))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD = %d; want 200", head.Code)
	}
	if got, ok := f.storedCaptured(tusIDOne); !ok || got != "2026-10-18T18:05:09" {
		t.Fatalf("after retry stored captured_at = %q, %v; want the first value", got, ok)
	}
}
